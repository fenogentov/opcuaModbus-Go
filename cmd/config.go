package main

import (
	"encoding/csv"
	"opcuaModbus/internal/clientopcua"
	"opcuaModbus/internal/modbus"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config ...
type Config struct {
	Logger  LoggerConf
	Devices DevicesConf
	Modbus  ModbusConf
}

// LoggerConf ...
type LoggerConf struct {
	Level, File string
}

// DevicesConf ...
type DevicesConf struct {
	Directory string
}

// ModbusConf ...
type ModbusConf struct {
	Host string
	Port int
}

// NewConfig is parsing config file.
func NewConfig(path string) (conf Config, err error) {
	if _, err := toml.DecodeFile(path, &conf); err != nil {
		return Config{}, err
	}
	return conf, nil
}

// readConfPlcs is reads PLCs config from tsv-file
func readConfPlcs(path string) (Plcs []clientopcua.DeviceOPCUA, err error) {
	file, err := os.Open(path + "/plc.tsv")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.Comment = '#'

	records, err := reader.ReadAll()
	if err != nil {
		return
	}

	for _, r := range records {
		if len(r) < 10 {
			continue
		}
		endpoint := "opc.tcp://" + strings.TrimSpace(r[2]) + ":" + strings.TrimSpace(r[3])
		policy := correctPolicy(r[4])
		mode := correctMode(r[5])
		auth := correctAuth(r[6])
		unitid := unitID(r[9])
		fileTags := path + "/" + strings.TrimSpace(r[10])

		plc := clientopcua.DeviceOPCUA{
			FileTags: fileTags,
			Status:   clientopcua.Configured,
			Config: clientopcua.Config{
				Endpoint: endpoint,
				Policy:   policy,
				Mode:     mode,
				Auth:     auth,
				Username: strings.TrimSpace(r[7]),
				Password: strings.TrimSpace(r[8]),
			},
			MBUnitID: unitid,
		}

		Plcs = append(Plcs, plc)
	}
	return Plcs, nil
}

// unitID is converts to type modbus.UnitID
func unitID(u string) modbus.UnitID {
	u = strings.TrimSpace(u)
	id, _ := strconv.Atoi(u)
	return modbus.UnitID(id)
}

// correctPolicy is correcting string Security Policy OPC UA
func correctPolicy(p string) string {
	p = strings.TrimSpace(p)
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

// correctMode is correcting string Security Mode OPC UA
func correctMode(m string) string {
	m = strings.TrimSpace(m)
	switch strings.ToLower(m) {
	case "sign":
		return "Sign"
	case "signandencrypt":
		return "SignAndEncrypt"
	default:
		return "None"
	}
}

// correctAuth is correcting string Authorization Mode OPC UA
func correctAuth(a string) string {
	a = strings.TrimSpace(a)
	switch strings.ToLower(a) {
	case "username":
		return "UserName"
	case "certificate":
		return "Certificate"
	default:
		return "Anonymous"
	}
}
