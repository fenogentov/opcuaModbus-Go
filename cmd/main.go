package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"opcuaModbus/internal/clientopcua"
	"opcuaModbus/internal/logger"
	"opcuaModbus/internal/modbus"
	"os"
	"os/signal"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
)

type serv struct {
	MBServer     *modbus.ModbusServer
	OPCUAClients clientopcua.DeviceOPCUA
}

var configFile string

func init() {
	flag.StringVar(&configFile, "config", "../opcuaModbus-Go/configs/config.toml", "path to configuration file")
}

func main() {
	flag.BoolVar(&debug.Enable, "debug", false, "enable debug logging")
	flag.Parse()

	Services := []clientopcua.DeviceOPCUA{}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// go func() {
	// 	<-ctx.Done()
	// 	stop()
	// 	log.Println(" shutdown signal received")
	// 	ctxTimeout, _ := context.WithTimeout(context.Background(), 5*time.Second)
	// 	<-ctxTimeout.Done()
	// 	log.Println(" stop ")
	// }()

	config, err := NewConfig(configFile)
	if err != nil {
		log.Fatalf("can't get config > %v", err)
	}

	logg := logger.New(config.Logger.File, config.Logger.Level)

	MBServer := modbus.NewServer(logg, "0.0.0.0", "1503")

	go MBServer.Listen()

	Services, err = CfgDevices(logg, config.Devices.Directory)
	if err != nil {
		logg.Error(err.Error())
	}

	// начальный запуск подписки клиентов opc ua
	for i := range Services {

		MBServer.AddDevice(Services[i].MBUnitID)

		err := Services[i].ClientOptions(ctx, logg)
		if err != nil {
			logg.Error(err.Error())
		}

		Services[i].Client = opcua.NewClient(Services[i].Config.Endpoint, Services[i].Options...)
		if err := Services[i].Client.Connect(ctx); err != nil {
			Services[i].Error = "Failed connect"
			continue
		}
		defer Services[i].Client.Close()

		Services[i].ReadTime(ctx)

		m, err := monitor.NewNodeMonitor(Services[i].Client)
		if err != nil {
			fmt.Println("err", err)
			continue
		}

		m.SetErrorHandler(func(c *opcua.Client, sub *monitor.Subscription, err error) {
			e := fmt.Sprintf("error: sub=%d err=%s", sub.SubscriptionID(), err)
			logg.Error(e)
		})

		Serv := &serv{
			MBServer:     MBServer,
			OPCUAClients: Services[i],
		}
		go startCallbackSub(ctx, m, Serv)
	}

	// запуск цикла мониторинга статуса клиентов opc ua
	ticker := time.NewTicker(time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				for i := range Services {
					fmt.Printf("Status: %s | Error: %s\n", Services[i].Status, Services[i].Error)

					if Services[i].Status == "CSV read" {
						MBServer.AddDevice(Services[i].MBUnitID)

						err := Services[i].ClientOptions(ctx, logg)
						if err != nil {
							logg.Error(err.Error())
						}
					}

					if Services[i].Status == "Configuration applied" {
						Services[i].Client = opcua.NewClient(Services[i].Config.Endpoint, Services[i].Options...)
						if err := Services[i].Client.Connect(ctx); err != nil {
							Services[i].Error = "Failed connect"
							continue
						}
						defer Services[i].Client.Close()

						Services[i].ReadTime(ctx)
					}
				}

				//case w := <-watcher.Events: // добавлен файл в папке

			}

		}
	}()

	// контроль папки с конфигурациями
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalln("wathing the directory with data files: ", err)
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case w := <-watcher.Events:
				fnames, err := filesNames(config.Devices.Directory)
				if err != nil {
					log.Println("error reading file list: ", err)
					continue
				}
				// add/del device opc ua
				fmt.Printf("%+v", w.Op)
				fmt.Println(fnames)
			case err := <-watcher.Errors:
				log.Fatalln("wathing the directory with data files: ", err)
			}
		}
	}()

	if err := watcher.Add(config.Devices.Directory); err != nil {
		log.Fatalln("wathing the directory with data files: ", err)
	}
	// ------------------------------------------

	<-ctx.Done()
}

func startCallbackSub(ctx context.Context, m *monitor.NodeMonitor, srvc *serv) {
	sub, err := m.Subscribe(
		ctx,
		nil,
		srvc.Hand,
		srvc.OPCUAClients.Nodes[0])

	if err != nil {
		fmt.Println(err)
		srvc.OPCUAClients.Status = "Error Subscribe"
	} else {
		go func() {
			<-ctx.Done()
			sub.Unsubscribe(ctx)
			srvc.OPCUAClients.Status = "Unsubscribe"
		}()
	}

	for i := 0; i < len(srvc.OPCUAClients.Nodes); i++ {
		err = sub.AddNodes(srvc.OPCUAClients.Nodes[i])
		if err != nil {
			fmt.Println(srvc.OPCUAClients.Nodes[i], err)
		}
	}
	srvc.OPCUAClients.Status = "Subscribe"
	fmt.Printf("%+v\n", sub.Subscribed())
	<-ctx.Done()
}

func (srv *serv) Hand(s *monitor.Subscription, msg *monitor.DataChangeMessage) {
	if msg.DataValue.Status != ua.StatusOK {
		log.Printf("[callback] sub=%d errorNodeID=%s", s.SubscriptionID(), msg.NodeID)
		return
	}
	unitid := srv.OPCUAClients.MBUnitID
	tag := srv.OPCUAClients.Tags[msg.NodeID.String()]

	switch tag.MBfunc {
	case modbus.ReadCoils:
		if val, ok := msg.Value.Value().(bool); ok {
			srv.MBServer.WriteCoils(unitid, tag.MBaddr, val)
			return
		}
		log.Println("err tag : ", msg.NodeID)

	case modbus.ReadDiscreteInputs:
		val := msg.Value.Value().(bool)
		srv.MBServer.WriteDiscreteInputs(unitid, tag.MBaddr, val)

	case modbus.ReadHoldingRegisters:
		regs := toRegisters(msg.Value.Value())
		for i, r := range regs {
			srv.MBServer.WriteHoldingRegisters(unitid, tag.MBaddr+uint16(i), r)
		}

	case modbus.ReadInputRegisters:
		regs := toRegisters(msg.Value.Value())
		for i, r := range regs {
			srv.MBServer.WriteInputRegisters(unitid, tag.MBaddr+uint16(i), r)
		}

	default:
	}
}

// toRegisters converting data to slice bytes
func toRegisters(v interface{}) (regs []uint16) {
	switch v := v.(type) {
	case byte:
		regs = append(regs, uint16(v))
	case int:
		regs = append(regs, uint16(v))
	case uint:
		regs = append(regs, uint16(v))
	case int8:
		regs = append(regs, uint16(v))
	case int16:
		regs = append(regs, uint16(v))
	case uint16:
		regs = append(regs, uint16(v))
	case int32:
		regs = append(regs, uint16(v>>16&0xFFFF))
		regs = append(regs, uint16(v&0xFFFF))
	case uint32:
		regs = append(regs, uint16(v>>16&0xFFFF))
		regs = append(regs, uint16(v&0xFFFF))
	case int64:
		regs = append(regs, uint16(v>>48&0xFFFF))
		regs = append(regs, uint16(v>>32&0xFFFF))
		regs = append(regs, uint16(v>>16&0xFFFF))
		regs = append(regs, uint16(v&0xFFFF))
	case uint64:
		regs = append(regs, uint16(v>>48&0xFFFF))
		regs = append(regs, uint16(v>>32&0xFFFF))
		regs = append(regs, uint16(v>>16&0xFFFF))
		regs = append(regs, uint16(v&0xFFFF))
	case float32:
		bits := math.Float32bits(v)
		regs = append(regs, uint16(bits>>16&0xFFFF))
		regs = append(regs, uint16(bits&0xFFFF))
	case float64:
		bits := math.Float64bits(v)
		regs = append(regs, uint16(bits>>48&0xFFFF))
		regs = append(regs, uint16(bits>>32&0xFFFF))
		regs = append(regs, uint16(bits>>16&0xFFFF))
		regs = append(regs, uint16(bits&0xFFFF))
	}

	return regs
}

// func clearString(s string) string {
// 	s = strings.TrimSpace(s)
// 	s = strings.ToLower(s)
// 	return s
// }
