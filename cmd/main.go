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

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/monitor"
	"github.com/sirupsen/logrus"

	"net/http"
	_ "net/http/pprof"
)

type serv struct {
	MBServer     *modbus.MBServer
	OPCUAClients *clientopcua.DeviceOPCUA
}

var configFile string

func init() {
	flag.StringVar(&configFile, "config", "./configs/config.toml", "path to configuration file")
}

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	config, err := NewConfig(configFile)
	if err != nil {
		log.Fatalf("can't get config: %v", err)
	}

	logg := logger.New(config.Logger.File, config.Logger.Level)

	MBServer := modbus.NewServer(logg, config.Modbus.Host, config.Modbus.Port)

	go MBServer.Listen()

	PLCs, err := readConfPlcs(config.Devices.Directory)
	if err != nil {
		logg.Error("error plc list: ", err)
		return
	}
	if len(PLCs) < 1 {
		logg.Error("error plc list: empty data")
		return
	}

	for i := range PLCs {
		MBServer.AddDevice(PLCs[i].MBUnitID)
	}

	ticker := time.NewTicker(1 * time.Minute)

	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				for i := range PLCs {

					if PLCs[i].Status == clientopcua.Configured {
						err := PLCs[i].ReadTagsTSV()
						if err != nil {
							PLCs[i].Error = "error read tsv"
							logg.Debug(PLCs[i].Config.Endpoint, " error: ", err)
						}
						logg.Debug(PLCs[i].Config.Endpoint, " status: ", PLCs[i].Status)
					}

					if PLCs[i].Status == clientopcua.ReadTags {
						err := PLCs[i].ClientOptions(ctx, logg)
						if err != nil {
							PLCs[i].Error = "error options"
							logg.Debug(PLCs[i].Config.Endpoint, " error: ", err)
						}
						logg.Debug(PLCs[i].Config.Endpoint, " status: ", PLCs[i].Status)
					}

					if PLCs[i].Status == clientopcua.ReadyOptions {
						client := opcua.NewClient(PLCs[i].Config.Endpoint, PLCs[i].Options...)
						PLCs[i].Client = client
						if err := PLCs[i].Client.Connect(ctx); err != nil {
							PLCs[i].Error = "failed connect"
							logg.Debug(PLCs[i].Config.Endpoint, " failed connect: ", err)
							continue
						}
						defer PLCs[i].Client.Close()
						PLCs[i].Status = clientopcua.Connected
						tm := PLCs[i].ReadTime(ctx)
						logg.Debug(PLCs[i].Config.Endpoint, " status: ", PLCs[i].Status, " /", tm)
					}

					if PLCs[i].Status == clientopcua.Connected {
						mntr, err := monitor.NewNodeMonitor(PLCs[i].Client)
						PLCs[i].Monitor = mntr
						if err != nil {
							logg.Debug(PLCs[i].Config.Endpoint, " error: ", err)
							continue
						}

						PLCs[i].Monitor.SetErrorHandler(func(c *opcua.Client, sub *monitor.Subscription, err error) {
							e := fmt.Sprintf("error: sub=%d err=%s", sub.SubscriptionID(), err)
							logg.Error(e)
						})

						Serv := &serv{
							MBServer:     MBServer,
							OPCUAClients: &PLCs[i],
						}

						go startCallbackSub(ctx, logg, Serv)
					}

					logg.Debug(PLCs[i].Config.Endpoint, " status: ", PLCs[i].Status, " / error: ", PLCs[i].Error)
					if PLCs[i].Subscrip != nil {
						logg.Debug(PLCs[i].Config.Endpoint, " subscribed ", PLCs[i].Subscrip.Subscribed(), " tags")
					}

					ticker.Reset(10 * time.Minute)
				}
			}
		}
	}()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	<-ctx.Done()
}

func startCallbackSub(ctx context.Context, logg *logrus.Logger, srvc *serv) {
	if len(srvc.OPCUAClients.Nodes) < 1 {
		srvc.OPCUAClients.Error = "empty Nodes"
		logg.Debug(srvc.OPCUAClients.Config.Endpoint + " Nodes empty")
		return
	}

	if srvc.OPCUAClients.Subscrip != nil {
		return
	}

	sub, err := srvc.OPCUAClients.Monitor.Subscribe(
		ctx,
		&opcua.SubscriptionParameters{
			Interval: 3 * time.Second,
		},
		srvc.handlerOPCUA,
		srvc.OPCUAClients.Nodes[0])
	if err != nil {
		srvc.OPCUAClients.Subscrip = nil
		srvc.OPCUAClients.Error = "error subscribe"
		logg.Error(srvc.OPCUAClients.Config.Endpoint, " error: ", err)
		return
	}
	srvc.OPCUAClients.Subscrip = sub
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe(ctx)
		srvc.OPCUAClients.Subscrip = nil
		srvc.OPCUAClients.Status = clientopcua.ReadyOptions
	}()

	for i := 1; i < len(srvc.OPCUAClients.Nodes); i++ {
		err = sub.AddNodes(srvc.OPCUAClients.Nodes[i])
		if err != nil {
			logg.Error(srvc.OPCUAClients.Config.Endpoint, "/", srvc.OPCUAClients.Nodes[i], " error: ", err)
		}
	}

	srvc.OPCUAClients.Status = clientopcua.Subscribed

	<-ctx.Done()
}

func (srv *serv) handlerOPCUA(s *monitor.Subscription, msg *monitor.DataChangeMessage) {
	unitid := srv.OPCUAClients.MBUnitID
	tag := srv.OPCUAClients.Tags[msg.NodeID.String()]
	val := msg.Value.Value()

	switch tag.MBfunc {
	case modbus.ReadCoils:
		if v, ok := val.(bool); ok {
			srv.MBServer.WriteCoils(unitid, tag.MBaddr, v)
			return
		}
		log.Println("err tag : ", msg.NodeID)

	case modbus.ReadDiscreteInputs:
		if v, ok := val.(bool); ok {
			srv.MBServer.WriteDiscreteInputs(unitid, tag.MBaddr, v)
			return
		}
		log.Println("err tag : ", msg.NodeID)

	case modbus.ReadHoldingRegisters:
		regs := toRegisters(val)
		for i, r := range regs {
			srv.MBServer.WriteHoldingRegisters(unitid, tag.MBaddr+uint16(i), r)
		}

	case modbus.ReadInputRegisters:
		regs := toRegisters(val)
		for i, r := range regs {
			srv.MBServer.WriteInputRegisters(unitid, tag.MBaddr+uint16(i), r)
		}

	default:
	}
}

// toRegisters is convert data to slice bytes
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
