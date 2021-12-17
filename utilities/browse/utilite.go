package utilities

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/pkg/errors"
)

var (
	Endpoint           string
	Policy, Auth, Mode string
	User, Password     string
	File               string
)

type NodeDef struct {
	NodeID      *ua.NodeID
	NodeClass   ua.NodeClass
	BrowseName  string
	Description string
	AccessLevel ua.AccessLevelType
	Path        string
	DataType    string
	Writable    bool
	Unit        string
	Scale       string
	Min         string
	Max         string
}

type NodeDefs []NodeDef

func (n NodeDefs) Len() int           { return len(n) }
func (n NodeDefs) Less(i, j int) bool { return n[i].DataType < n[j].DataType }
func (n NodeDefs) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

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

	// browse tags
	id, err := ua.ParseNodeID("ns=0;i=84")
	if err != nil {
		log.Fatalf("invalid node id: %s", err)
	}
	nodeList, err := browse(clnt.Node(id), "", 0)
	if err != nil {
		fmt.Println(err)
		return
	}
	sort.Sort(NodeDefs(nodeList))

	file.WriteString("#\n#\n")
	w := csv.NewWriter(file)
	w.Comma = ','
	hdr := []string{"#", "Tag Name", "Address", "Data Type", "Function MB", "Address MB"}
	w.Write(hdr)
	coil := 1
	holding := 1
	cnt := 1
	for _, s := range nodeList {
		var rec []string
		switch s.DataType {
		case "bool":
			rec = s.records(&cnt, "Coil", coil)
			coil++
		case "time.Time": // "string"
			rec = s.records(&cnt, "Holding", holding)
			holding += 50
		case "byte", "int", "uint", "int8", "uint8", "int16", "uint16":
			rec = s.records(&cnt, "Holding", holding)
			holding++
		case "int32", "uint32", "float32", "float":
			rec = s.records(&cnt, "Holding", holding)
			holding += 2
		case "int64", "uint64", "float64", "double":
			rec = s.records(&cnt, "Holding", holding)
			holding += 4
		default:
			continue
		}
		w.Write(rec)
	}
	w.Flush()
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

func browse(n *opcua.Node, path string, level int) ([]NodeDef, error) {
	// fmt.Printf("node:%s path:%q level:%d\n", n, path, level)
	if level > 10 {
		return nil, nil
	}

	attrs, err := n.Attributes(ua.AttributeIDNodeClass, ua.AttributeIDBrowseName, ua.AttributeIDDescription, ua.AttributeIDAccessLevel, ua.AttributeIDDataType)
	if err != nil {
		return nil, err
	}

	var def = NodeDef{
		NodeID: n.ID,
	}

	switch err := attrs[0].Status; err {
	case ua.StatusOK:
		def.NodeClass = ua.NodeClass(attrs[0].Value.Int())
	default:
		return nil, err
	}

	switch err := attrs[1].Status; err {
	case ua.StatusOK:
		def.BrowseName = attrs[1].Value.String()
	default:
		return nil, err
	}

	switch err := attrs[2].Status; err {
	case ua.StatusOK:
		def.Description = attrs[2].Value.String()
	case ua.StatusBadAttributeIDInvalid:
		// ignore
	default:
		return nil, err
	}

	switch err := attrs[3].Status; err {
	case ua.StatusOK:
		def.AccessLevel = ua.AccessLevelType(attrs[3].Value.Int())
		def.Writable = def.AccessLevel&ua.AccessLevelTypeCurrentWrite == ua.AccessLevelTypeCurrentWrite
	case ua.StatusBadAttributeIDInvalid:
		// ignore
	default:
		return nil, err
	}

	switch err := attrs[4].Status; err {
	case ua.StatusOK:
		switch v := attrs[4].Value.NodeID().IntID(); v {
		case id.DateTime:
			def.DataType = "time.Time"
		case id.Boolean:
			def.DataType = "bool"
		case id.SByte:
			def.DataType = "int8"
		case id.Int16:
			def.DataType = "int16"
		case id.Int32:
			def.DataType = "int32"
		case id.Int64:
			def.DataType = "int64"
		case id.Byte:
			def.DataType = "byte"
		case id.UInt16:
			def.DataType = "uint16"
		case id.UInt32:
			def.DataType = "uint32"
		case id.UInt64:
			def.DataType = "uint64"
		case id.UtcTime:
			def.DataType = "time.Time"
		case id.String:
			def.DataType = "string"
		case id.Float:
			def.DataType = "float32"
		case id.Double:
			def.DataType = "float64"
		default:
			def.DataType = attrs[4].Value.NodeID().String()
		}
	case ua.StatusBadAttributeIDInvalid:
		// ignore
	default:
		return nil, err
	}

	def.Path = join(path, def.BrowseName)
	// fmt.Printf("%d: def.Path:%s def.NodeClass:%s\n", level, def.Path, def.NodeClass)

	var nodes []NodeDef
	if def.NodeClass == ua.NodeClassVariable {
		nodes = append(nodes, def)
	}

	browseChildren := func(refType uint32) error {
		refs, err := n.ReferencedNodes(refType, ua.BrowseDirectionForward, ua.NodeClassAll, true)
		if err != nil {
			return errors.Errorf("References: %d: %s", refType, err)
		}
		// fmt.Printf("found %d child refs\n", len(refs))
		for _, rn := range refs {
			children, err := browse(rn, def.Path, level+1)
			if err != nil {
				return errors.Errorf("browse children: %s", err)
			}
			nodes = append(nodes, children...)
		}
		return nil
	}

	if err := browseChildren(id.HasComponent); err != nil {
		return nil, err
	}
	if err := browseChildren(id.Organizes); err != nil {
		return nil, err
	}
	if err := browseChildren(id.HasProperty); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (n NodeDef) records(i *int, mbfunc string, addr int) []string {
	cnt := strconv.Itoa(*i)
	mbaddr := strconv.Itoa(addr)
	*i++
	return []string{cnt, n.BrowseName, n.NodeID.String(), n.DataType, mbfunc, mbaddr}
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

func recordEnpointParam(endpoints []*ua.EndpointDescription) error {
	fout := strings.TrimSuffix(File, ".csv") + "_out.csv"
	f, err := os.Create(fout)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("#OPCUA Endpoint: " + Endpoint + "\n")
	if err != nil {
		return err
	}
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
