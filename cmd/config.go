package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"opcuaModbus/internal/clientopcua"
	"opcuaModbus/internal/logger"
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
}

// LoggerConf ...
type LoggerConf struct {
	Level, File string
}

// DevicesConf ...
type DevicesConf struct {
	Directory string
}

// NewConfig parsing config file.
func NewConfig(path string) (conf Config, err error) {
	if _, err := toml.DecodeFile(path, &conf); err != nil {
		return Config{}, err
	}
	return conf, nil
}

// CfgDevices reads configuration for service structures
func CfgDevices(logg *logger.Logger, path string) (dvc []clientopcua.DeviceOPCUA, err error) {

	fnames, err := filesNames(path)
	if err != nil {
		return dvc, err
	}

	for _, file := range fnames {
		fullName := path + "/" + file.Name()

		// read parameters OPC UA
		srv, err := readDeviceOPCUA(fullName)
		if err != nil {
			srv.Error = "Error configuration"
			logg.Error("error read parameters file " + fullName + ": " + err.Error())
			continue
		}
		srv.Status = "Configuration read"
		t := fmt.Sprintf("Config: %+v", srv.Config)
		logg.Info("read file " + fullName)
		logg.Debug(t)

		// read tags OPC UA
		srv.Nodes, srv.Tags, err = readCSV(fullName, 2, 3, 4, 5)
		if err != nil {
			srv.Error = "Error read CSV"
			logg.Error("error read csv-file " + fullName + ": " + err.Error())
			continue
		}

		srv.Status = "CSV read"
		dvc = append(dvc, srv)
	}

	return dvc, nil
}

// filesNames getting information about files in a directory
func filesNames(path string) (fileInfo []fs.FileInfo, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fileName := f.Name()
	f.Close()
	fileInfo, err = ioutil.ReadDir(fileName)
	if err != nil {
		return nil, err
	}
	return fileInfo, err
}

func readDeviceOPCUA(filename string) (dvc clientopcua.DeviceOPCUA, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return dvc, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	cnt := 0
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
					return dvc, errors.New("error endpoint" + strings.Join(sl[1:], ":"))
				}
				dvc.Config.Endpoint = strings.TrimSpace(sl[1] + ":" + sl[2] + ":" + sl[3])
			case strings.Contains(p, "policy"):
				dvc.Config.Policy = strings.TrimSpace(sl[1])
			case strings.Contains(p, "security mode"):
				dvc.Config.Mode = strings.TrimSpace(sl[1])
			case strings.Contains(p, "auth"):
				dvc.Config.Auth = strings.TrimSpace(sl[1])
			case strings.Contains(p, "user"):
				dvc.Config.Username = strings.TrimSpace(sl[1])
			case strings.Contains(p, "pass"):
				dvc.Config.Password = strings.TrimSpace(sl[1])
			// case strings.Contains(p, "port"):
			// 	devMB.Port = sl[1]
			case strings.Contains(p, "unitid"):
				sl[1] = strings.TrimSpace(sl[1])
				id, err := strconv.Atoi(sl[1])
				if err != nil {
					return dvc, fmt.Errorf("error ModBus UnitID: %v", err)
				}
				dvc.MBUnitID = modbus.UnitID(id)
			}
		}
		if cnt == 20 {
			break
		}

		cnt++
	}
	if dvc.MBUnitID < 1 || dvc.MBUnitID > 247 {
		return dvc, fmt.Errorf("error ModBus UnitID: %v", dvc.MBUnitID)
	}
	if err := scanner.Err(); err != nil {
		return dvc, err
	}
	if dvc.Config.Endpoint == "" {
		return dvc, errors.New("#OPCUA Endpoint cannot be empty")
	}
	dvc.Config.Policy = correctPolicy(dvc.Config.Policy)
	dvc.Config.Mode = correctMode(dvc.Config.Mode)
	if (dvc.Config.Mode == "None" && dvc.Config.Policy != "None") || (dvc.Config.Mode != "None" && dvc.Config.Policy == "None") {
		return dvc, fmt.Errorf("incorrect configuration of security Policy / Mode: %s / %s", dvc.Config.Policy, dvc.Config.Mode)
	}

	dvc.Config.Auth = correctAuth(dvc.Config.Auth)
	if dvc.Config.Auth == "UserName" && (dvc.Config.Username == "" || dvc.Config.Password == "") {
		return dvc, errors.New("#OPCUA Authentication mode: UserName requires a Username and Password ")
	}

	return dvc, nil
}

// readCSV parse tags OPC UA from csv-file
// readCSV (filename string, arg1 int, arg2 int, arg3 int, arg4 int)
// arg1 - номер колонки Address (NodeID OPC UA)
// arg2 - номер колонки типа данных (float, bool, тд)
// arg3 - номер колонки функции модбас (holding, coil ...)/(1,2,3,4,5...)
// arg4 - номер колонки адреса модбас
func readCSV(filename string, arg ...int) (nodes []string, tags map[string]clientopcua.Tag, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.Comment = '#'

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	tgs := make(map[string]clientopcua.Tag)
	for _, r := range records {
		if len(r) != 6 {
			continue
		}
		tg := clientopcua.Tag{}
		name := r[arg[0]]
		tg.TypeData = r[arg[1]]
		tg.MBfunc = modbus.StringToUint8(r[arg[2]])
		a, err := strconv.Atoi(r[arg[3]])
		if err != nil {
			fmt.Println(err)
			continue
		}
		tg.MBaddr = uint16(a)
		nodes = append(nodes, name)
		tgs[name] = tg
	}
	if len(nodes) == 0 || len(tgs) == 0 {
		return nil, nil, errors.New("empty data csv")
	}
	return nodes, tgs, nil
}

// correctPolicy correcting string Security Policy OPC UA
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

// correctPolicy correcting string Security Mode OPC UA
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

// correctPolicy correcting string Mode Authorization OPC UA
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
