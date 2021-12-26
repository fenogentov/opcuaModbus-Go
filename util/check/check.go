package utilities

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
)

var (
	Endpoint           string
	Policy, Auth, Mode string
	User, Password     string
	File               string
)

func init() {
	flag.StringVar(&File, "f", "", "file name")
}

func main() {
	flag.Parse()

	if File == "" {
		fmt.Println("ERROR:")
		fmt.Println("flags -f must be specified csv file name")
		return
	}

	// read config file *.csv
	file, err := os.OpenFile(File, os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		fmt.Println("ERROR:")
		fmt.Println(err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		l := scanner.Text()
		l = strings.ReplaceAll(l, ",", "")
		if !strings.Contains(l, "#") {
			continue
		}
		sl := strings.Split(l, ":")

		if len(sl) > 1 {
			p := strings.ToLower(sl[0])
			switch {
			case strings.Contains(p, "endpoint"):
				if len(sl) != 4 {
					fmt.Println("ERROR:")
					fmt.Println(l)
					return
				}
				Endpoint = strings.TrimSpace(sl[1] + ":" + sl[2] + ":" + sl[3])
			case strings.Contains(p, "policy"):
				Policy = strings.TrimSpace(sl[1])
			case strings.Contains(p, "security mode"):
				Mode = strings.TrimSpace(sl[1])
			case strings.Contains(p, "auth"):
				Auth = strings.TrimSpace(sl[1])
			case strings.Contains(p, "user"):
				User = strings.TrimSpace(sl[1])
			case strings.Contains(p, "pass"):
				Password = strings.TrimSpace(sl[1])
			}
		}
	}
	// correcting config
	if Endpoint == "" {
		fmt.Println("ERROR:")
		fmt.Println("Endpoint must be specified")
		fmt.Println("e.g. \"#OPCUA Endpoint: opc.tcp://10.0.0.10:48480\"")
		return
	}

	Policy = correctPolicy(Policy)
	Mode = correctMode(Mode)
	if (Mode == "None" && Policy != "None") || (Mode != "None" && Policy == "None") {
		fmt.Println("ERROR:")
		fmt.Printf("Incorrect configuration of security Policy / Mode: %s / %s", Policy, Mode)
		return
	}

	Auth = correctAuth(Auth)
	if Auth == "UserName" && User == "" {
		fmt.Print("Enter username: ")
		User, err = bufio.NewReader(os.Stdin).ReadString('\n')
		User = strings.TrimSuffix(User, "\n")
		if err != nil {
			log.Fatalf("error reading username input: %s", err)
		}
	}
	if Auth == "UserName" && Password == "" {
		fmt.Print("Enter password: ")
		Password, err = bufio.NewReader(os.Stdin).ReadString('\n')
		Password = strings.TrimSuffix(Password, "\n")
		if err != nil {
			log.Fatalf("error reading passowr input: %s", err)
		}
	}

	fmt.Printf("Config:\tEndpoins: %s\n\tSecurity Mode: %s, %s\n\tAuth Mode: %s\n\tUserName: %s\n\tPassword: %s\n", Endpoint, Policy, Mode, Auth, User, Password)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	// Apply config
	endpoints, err := opcua.GetEndpoints(ctx, Endpoint)
	if err != nil {
		log.Fatal(err)
	}

	endpnt := opcua.SelectEndpoint(endpoints, Policy, ua.MessageSecurityModeFromString(Mode))
	if endpnt == nil {
		fmt.Println("Failed to find suitable endpoint")
		err := recordEnpointParam(endpoints)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	opts := []opcua.Option{}
	opts = append(opts, opcua.AutoReconnect(false))

	opts = append(opts, opcua.SecurityPolicy(Policy))
	opts = append(opts, opcua.SecurityModeString(Mode))
	if Policy != "None" {
		opts = append(opts, opcua.CertificateFile("cert.pem"))
		opts = append(opts, opcua.PrivateKeyFile("key.pem"))
	}

	var authToken ua.UserTokenType
	switch Auth {
	case "UserName":
		authToken = ua.UserTokenTypeUserName
		opts = append(opts, opcua.AuthUsername(User, Password))
	case "Certificate":
		authToken = ua.UserTokenTypeCertificate
		//		opts = append(opts, opcua.AuthCertificate(cert))
	default:
		authToken = ua.UserTokenTypeAnonymous
		opts = append(opts, opcua.AuthAnonymous())
	}
	opts = append(opts, opcua.SecurityFromEndpoint(endpnt, authToken))

	// Test Read time server
	fmt.Println("--------------------------------------------------")
	clnt := opcua.NewClient(Endpoint, opts...)
	if err := clnt.Connect(ctx); err != nil {
		fmt.Println(err)
		err := recordEnpointParam(endpoints)
		if err != nil {
			fmt.Println(err)
		}
		return
	}
	defer clnt.Close()

	vl, err := clnt.Node(ua.NewNumericNodeID(0, 2258)).Value()
	if err != nil {
		fmt.Println("err : ", err)
	}
	if vl != nil {
		fmt.Printf("Server's time: %s\n", vl.Value())
	} else {
		fmt.Print("v == nil")
	}
	fmt.Println("--------------------------------------------------")

	m, err := monitor.NewNodeMonitor(clnt)
	if err != nil {
		log.Fatal(err)
	}

	m.SetErrorHandler(func(_ *opcua.Client, sub *monitor.Subscription, err error) {
		log.Printf("error: sub=%d err=%s", sub.SubscriptionID(), err.Error())
	})

	file.Seek(0, 0)
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.Comment = '#'

	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println(err)
		return
	}

	sub, err := m.Subscribe(
		ctx,
		nil,
		Hand,
		"ns=3;i=1001")
	if err != nil {
		fmt.Println(err)
		return
	}

	for i, r := range records {
		if len(r) != 6 {
			fmt.Println(i, ". len != 6")
			continue
		}

		err = sub.AddNodes(r[2])
		if err != nil {
			fmt.Println(err)
		}
	}
	<-ctx.Done()
	sub.Unsubscribe(ctx)
}
func Hand(s *monitor.Subscription, msg *monitor.DataChangeMessage) {
	if msg.Error != nil {
		log.Printf("[callback] sub=%d error=%s", s.SubscriptionID(), msg.Error)
	} else {
		log.Printf("[callback] sub=%d ts=%s node=%s value=%v", s.SubscriptionID(), msg.SourceTimestamp.UTC().Format(time.RFC3339), msg.NodeID, msg.Value.Value())
	}
}

func correctPolicy(p string) string {
	switch strings.ToLower(p) {
	case "basic128rsa15":
		return "Basic128Rsa15"
	case "basic256":
		return "Basic256"
	case "basic256sha256":
		return "Basic256Sha256"
	case "aes128_sha256_rsaoaep":
		return "Aes128_Sha256_RsaOaep"
	case "aes256_sha256_rsapss":
		return "Aes256_Sha256_RsaPss"
	default:
		return "None"
	}
}
func correctMode(m string) string {
	switch strings.ToLower(m) {
	case "sign":
		return "Sign"
	case "signandencrypt":
		return "SignAndEncrypt"
	default:
		return "None"
	}
}
func correctAuth(a string) string {
	switch strings.ToLower(a) {
	case "username":
		return "UserName"
	case "certificate":
		return "Certificate"
	default:
		return "Anonymous"
	}
}

func recordEnpointParam(endpoints []*ua.EndpointDescription) error {
	fout := strings.TrimSuffix(File, ".csv") + "_out.csv"
	f, err := os.Create(fout)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("#OPCUA Endpoint: " + Endpoint + "\n")
	enp := getOptions(endpoints)
	f.WriteString(enp)
	fmt.Printf("created file " + fout + "\n")

	return nil
}

func getOptions(endpoints []*ua.EndpointDescription) (out string) {
	var policy, mode, auth []string
	var user bool
	for _, e := range endpoints {
		p := strings.TrimPrefix(e.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#")
		if !findFromSlice(policy, p) {
			policy = append(policy, p)
		}
		m := strings.TrimPrefix(e.SecurityMode.String(), "MessageSecurityMode")
		if !findFromSlice(mode, m) {
			mode = append(mode, m)
		}
		for _, t := range e.UserIdentityTokens {
			tok := strings.TrimPrefix(t.TokenType.String(), "UserTokenType")
			if !findFromSlice(auth, tok) {
				auth = append(auth, tok)
				if tok == "UserName" {
					user = true
				}
			}
		}
	}
	if len(policy) > 0 {
		out = out + "#OPCUA Security Policy: " + strings.Join(policy, "/") + "\n"
	}
	if len(mode) > 0 {
		out = out + "#OPCUA Security Mode: " + strings.Join(mode, "/") + "\n"
	}
	if len(auth) > 0 {
		out = out + "#OPCUA Auth Mode: " + strings.Join(auth, "/") + "\n"
	}
	if user {
		out = out + "#OPCUA UserName: \n#OPCUA Passord: "
	}

	return out
}

func findFromSlice(sl []string, e string) bool {
	for _, s := range sl {
		if s == e {
			return true
		}
	}
	return false
}
