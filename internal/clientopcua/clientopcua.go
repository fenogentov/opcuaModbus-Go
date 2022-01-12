package clientopcua

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"opcuaModbus/internal/logger"
	"opcuaModbus/internal/modbus"
	"opcuaModbus/utilities"
	"os"
	"strconv"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
)

type Status int

const (
	Configured Status = iota + 1
	ReadTags
	ReadyOptions
	Connected
	Subscribed
)

// Tag is config for tags device
type Tag struct {
	TypeData string
	MBfunc   uint8
	MBaddr   uint16
}

// Config is configuration of connection to OPC UA Server
type Config struct {
	Endpoint string
	Policy   string
	Mode     string
	Auth     string
	Username string
	Password string
}

// DeviceOPCUA is client OPC UA
type DeviceOPCUA struct {
	Status   Status
	Config   Config
	Client   *opcua.Client
	Options  []opcua.Option
	client   *opcua.Client
	Monitor  *monitor.NodeMonitor
	Subscrip *monitor.Subscription
	Nodes    []string
	Tags     map[string]Tag
	MBUnitID modbus.UnitID
	Error    string
	FileTags string
}

// ClientOptions is applying OPC UA Client connection configuration
func (dvc *DeviceOPCUA) ClientOptions(ctx context.Context, logg *logger.Logger) error {
	endpoints, err := opcua.GetEndpoints(ctx, dvc.Config.Endpoint)
	if err != nil {
		return fmt.Errorf("error GetEndoints: %s", err)
	}

	endpnt := opcua.SelectEndpoint(endpoints, dvc.Config.Policy, ua.MessageSecurityModeFromString(dvc.Config.Mode))
	if endpnt == nil {
		recordEnpointParam(endpoints)
	}

	dvc.Options = append(dvc.Options, opcua.AutoReconnect(true))

	dvc.Options = append(dvc.Options, opcua.SecurityPolicy(dvc.Config.Policy))
	dvc.Options = append(dvc.Options, opcua.SecurityModeString(dvc.Config.Mode))
	if dvc.Config.Policy != "None" {
		dvc.Options = append(dvc.Options, opcua.CertificateFile("cert.pem"))
		dvc.Options = append(dvc.Options, opcua.PrivateKeyFile("key.pem"))
	}

	var authToken ua.UserTokenType
	switch dvc.Config.Auth {
	case "UserName":
		authToken = ua.UserTokenTypeUserName
		dvc.Options = append(dvc.Options, opcua.AuthUsername(dvc.Config.Username, dvc.Config.Password))
	case "Certificate":
		authToken = ua.UserTokenTypeCertificate
		//		opts = append(opts, opcua.AuthCertificate(cert))
	default:
		authToken = ua.UserTokenTypeAnonymous
		dvc.Options = append(dvc.Options, opcua.AuthAnonymous())
	}
	dvc.Options = append(dvc.Options, opcua.SecurityFromEndpoint(endpnt, authToken))

	dvc.Status = ReadyOptions
	return nil
}

func recordEnpointParam(endpoints []*ua.EndpointDescription) {
	enp := getOptions(endpoints)
	fmt.Println(enp)
}

// getOptions getting configuration of connection to OPC UA Server
func getOptions(endpoints []*ua.EndpointDescription) (out string) {
	var policy, mode, auth []string
	var user bool
	for _, e := range endpoints {
		p := strings.TrimPrefix(e.SecurityPolicyURI, "http://opcfoundation.org/UA/SecurityPolicy#")
		if !utilities.FindFromSliceString(policy, p) {
			policy = append(policy, p)
		}
		m := strings.TrimPrefix(e.SecurityMode.String(), "MessageSecurityMode")
		if !utilities.FindFromSliceString(mode, m) {
			mode = append(mode, m)
		}
		for _, t := range e.UserIdentityTokens {
			token := strings.TrimPrefix(t.TokenType.String(), "UserTokenType")
			if !utilities.FindFromSliceString(auth, token) {
				auth = append(auth, token)
				if token == "UserName" {
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

// readTime is tests the connection and reads Server's Time
func (dvc *DeviceOPCUA) ReadTime(ctx context.Context) {

	vl, err := dvc.Client.Node(ua.NewNumericNodeID(0, 2258)).Value()
	if err != nil {
		fmt.Println("err : ", err)
		return
	}
	if vl != nil {
		fmt.Printf("Server's time: %s\n", vl.Value())
		dvc.Status = Connected
	} else {
		fmt.Print("v == nil")
		dvc.Error = "Failed connect"
	}
}

func (dvc *DeviceOPCUA) ReadTagsTSV() error {
	file, err := os.Open(dvc.FileTags)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.Comment = '#'

	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	tags := make(map[string]Tag)
	nodes := []string{}
	for _, r := range records {
		if len(r) != 6 {
			continue
		}
		tg := Tag{}
		name := r[2]
		tg.TypeData = r[3]
		tg.MBfunc = modbus.StringToUint8(r[4])
		a, err := strconv.Atoi(r[5])
		if err != nil {
			continue
		}
		tg.MBaddr = uint16(a)
		nodes = append(nodes, name)
		tags[name] = tg
	}
	if len(nodes) == 0 || len(tags) == 0 {
		return errors.New("empty data tsv")
	}

	dvc.Nodes = append(dvc.Nodes, nodes...)
	dvc.Tags = tags
	dvc.Status = ReadTags

	return nil
}
