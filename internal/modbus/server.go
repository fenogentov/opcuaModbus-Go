package modbus

import (
	"encoding/binary"
	"net"
	"opcuaModbus/utilities"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MBServer ..
type MBServer struct {
	host        string
	Port        string
	IdleTimeout time.Duration
	tcpListener net.Listener
	Devices     map[UnitID]MBData
	logg        *logrus.Logger
}

// NewServer creating a new modbus server
func NewServer(logg *logrus.Logger, host string, port int) *MBServer {
	if host == "" {
		host = "0.0.0.0"
	}
	prt := strconv.Itoa(port)

	return &MBServer{
		host:        host,
		Port:        prt,
		IdleTimeout: 30 * time.Second,
		Devices:     make(map[UnitID]MBData),
		logg:        logg,
	}
}

// AddDevice adding a device with a given modbus address to the modbus server
func (server *MBServer) AddDevice(id UnitID) {
	if _, ok := server.Devices[id]; ok {
		return
	}
	server.Devices[id] = MBData{
		RWCoils:            &sync.RWMutex{},
		RWDiscreteInputs:   &sync.RWMutex{},
		RWHoldingRegisters: &sync.RWMutex{},
		RWInputRegisters:   &sync.RWMutex{},
		Coils:              map[uint16]bool{},
		DiscreteInputs:     map[uint16]bool{},
		HoldingRegisters:   map[uint16]uint16{},
		InputRegisters:     map[uint16]uint16{},
	}
	server.logg.Info("modbus server add unit: ", id)
}

// раскидать listen и accept
func (server *MBServer) Listen() {
	var err error

	url := server.host + ":" + server.Port
	server.tcpListener, err = net.Listen("tcp", url)
	if err != nil {
		server.logg.Error(err.Error())
		return
	}
	server.logg.Info("modbus server listen: ", url)
	defer server.tcpListener.Close()

	for {
		sock, err := server.tcpListener.Accept()
		if err != nil {
			server.logg.Error("failed to accept client connection: ", err)
			continue
		}
		go server.handlerMB(sock)
	}
}

// handlerMB is request handler for ModBus Server
func (server *MBServer) handlerMB(sock net.Conn) {
	defer func() {
		server.logg.Debug("modbus server close socket: ", sock.RemoteAddr())
		sock.Close()
	}()

	for {
		packet := make([]byte, 512)
		bytesRead, err := sock.Read(packet)
		if err != nil {
			server.logg.Error("socket read error: ", err, " / ", sock.RemoteAddr())
			return
		}
		err = sock.SetDeadline(time.Now().Add(server.IdleTimeout))
		if err != nil {
			server.logg.Error("socket set deadline error: ", err, " / ", sock.RemoteAddr())
			return
		}

		packet = packet[:bytesRead]
		if len(packet) < 12 || len(packet) > 260 {
			server.logg.Debug("modbus exception: bad len packet / ", sock.RemoteAddr())
			return
		}

		transactionID := binary.BigEndian.Uint16(packet[0:2])
		protocolID := binary.BigEndian.Uint16(packet[2:4])
		unitid := UnitID(packet[6])
		function := uint8(packet[7])
		startingAddress := binary.BigEndian.Uint16(packet[8:10])
		quantity := binary.BigEndian.Uint16(packet[10:12])

		response := &mbResponse{
			transactionID: transactionID,
			protocolID:    protocolID,
			UnitID:        unitid,
			function:      function,
		}

		exception := Success
		if unitid > 247 {
			exception = SlaveDeviceFailure
		}
		if _, ok := server.Devices[unitid]; !ok {
			exception = SlaveDeviceFailure
		}

		if exception == Success {
			switch function {
			case ReadCoils:
				if quantity < 1 || quantity > 2000 || (startingAddress+quantity) > 65535 {
					exception = IllegalDataValue
					break
				}
				exception = server.readCoils(response, startingAddress, quantity)

			case ReadDiscreteInputs:
				if quantity < 1 || quantity > 2000 || (startingAddress+quantity) > 65535 {
					exception = IllegalDataValue
					break
				}
				exception = server.readDiscreteInputs(response, startingAddress, quantity)

			case ReadHoldingRegisters:
				if quantity < 1 || quantity > 2000 || (startingAddress+quantity) > 65535 {
					exception = IllegalDataValue
					break
				}
				exception = server.readHoldingRegister(response, startingAddress, quantity)
			case ReadInputRegisters:
				if quantity < 1 || quantity > 2000 || (startingAddress+quantity) > 65535 {
					exception = IllegalDataValue
					break
				}
				exception = server.readInputRegisters(response, startingAddress, quantity)

			case WriteSingleCoil:
			case WriteMultipleCoils:
			case WriteSingleRegister:
			case WriteMultipleRegisters:

			default:
				exception = IllegalFunction
			}
		}

		if exception != Success {
			response.sendExeption(sock, exception)
			server.logg.Debug("modbus send exception: ", exception, " / ", sock.RemoteAddr())
			continue
		}

		response.sendData(sock)
	}
}

// sendExeption is create response with ModBus exception on error
func (r *mbResponse) sendExeption(sock net.Conn, ex Exception) {
	bytes := make([]byte, 2)
	rawBytes := []byte{}
	binary.BigEndian.PutUint16(bytes, r.transactionID)
	rawBytes = append(rawBytes, bytes...)
	binary.BigEndian.PutUint16(bytes, r.protocolID)
	rawBytes = append(rawBytes, bytes...)
	binary.BigEndian.PutUint16(bytes, uint16(3))
	rawBytes = append(rawBytes, bytes...)
	rawBytes = append(rawBytes, byte(r.UnitID))
	rawBytes = append(rawBytes, (r.function | 0x80))
	rawBytes = append(rawBytes, uint8(ex))
	_, _ = sock.Write(rawBytes)
}

// sendData is create response with ModBus data
func (r *mbResponse) sendData(sock net.Conn) {
	bytes := make([]byte, 2)
	rawBytes := []byte{}
	binary.BigEndian.PutUint16(bytes, r.transactionID)
	rawBytes = append(rawBytes, bytes...)
	binary.BigEndian.PutUint16(bytes, r.protocolID)
	rawBytes = append(rawBytes, bytes...)
	r.lenght = uint16(len(r.Data) + 2)
	binary.BigEndian.PutUint16(bytes, r.lenght)
	rawBytes = append(rawBytes, bytes...)
	rawBytes = append(rawBytes, byte(r.UnitID))
	rawBytes = append(rawBytes, r.function)
	rawBytes = append(rawBytes, r.Data...)
	_, _ = sock.Write(rawBytes)
}

// func (resp *ResponseMB)  WriteSingleCoil
// func (resp *ResponseMB)  WriteHoldingRegister
// func (resp *ResponseMB)  WriteMultipleCoils
// func (resp *ResponseMB)  WriteHoldingRegisters

// readCoils is read Coils data in ModBus Server & send response
func (server *MBServer) readCoils(r *mbResponse, startAddress, quantity uint16) Exception {
	bts := []byte{}
	buff := []bool{}
	var i uint16
	server.Devices[r.UnitID].RWCoils.RLock()
	defer server.Devices[r.UnitID].RWCoils.RUnlock()
	for i = startAddress; i < (startAddress + quantity); i++ {
		if _, ok := server.Devices[r.UnitID]; !ok {
			return IllegalDataAddress
		}
		if _, ok := server.Devices[r.UnitID].Coils[i]; !ok {
			return IllegalDataAddress
		}
		buff = append(buff, server.Devices[r.UnitID].Coils[i])
	}

	for i := 0; i < len(buff); i += 8 {
		var b byte
		for j := 0; j < 8 && (i+j) < len(buff); j++ {
			utilities.SetBit(&b, j, buff[i])
		}
		bts = append(bts, b)
	}

	r.Data = append(r.Data, byte(len(bts)))
	r.Data = append(r.Data, bts...)
	return Success
}

// readDiscreteInputs is read Discrete inputs data in ModBus Server & send response
func (server *MBServer) readDiscreteInputs(r *mbResponse, startAddress, quantity uint16) Exception {
	bts := []byte{}
	buff := []bool{}
	var i uint16
	server.Devices[r.UnitID].RWDiscreteInputs.RLock()
	defer server.Devices[r.UnitID].RWDiscreteInputs.RUnlock()
	for i = startAddress; i < (startAddress + quantity); i++ {
		if _, ok := server.Devices[r.UnitID]; !ok {
			return IllegalDataAddress
		}
		if _, ok := server.Devices[r.UnitID].DiscreteInputs[i]; !ok {
			return IllegalDataAddress
		}
		buff = append(buff, server.Devices[r.UnitID].DiscreteInputs[i])
	}

	for i := 0; i < len(buff); i += 8 {
		var b byte
		for j := 0; j < 8 && (i+j) < len(buff); j++ {
			utilities.SetBit(&b, j, buff[i])
		}
		bts = append(bts, b)
	}

	r.Data = append(r.Data, byte(len(bts)))
	r.Data = append(r.Data, bts...)
	return Success
}

// readDiscreteInputs is read Holding registers data in ModBus Server & send response
func (server *MBServer) readHoldingRegister(r *mbResponse, startAddress, quantity uint16) Exception {
	register := make([]byte, 2)
	buff := []byte{}
	var i uint16
	server.Devices[r.UnitID].RWHoldingRegisters.RLock()
	defer server.Devices[r.UnitID].RWHoldingRegisters.RUnlock()
	for i = startAddress; i < (startAddress + quantity); i++ {
		if _, ok := server.Devices[r.UnitID]; !ok {
			return IllegalDataAddress
		}
		if _, ok := server.Devices[r.UnitID].HoldingRegisters[i]; !ok {
			return IllegalDataAddress
		}
		binary.BigEndian.PutUint16(register, server.Devices[r.UnitID].HoldingRegisters[i])
		buff = append(buff, register...)
	}
	r.Data = append(r.Data, byte(len(buff)))
	r.Data = append(r.Data, buff...)
	return Success
}

// readDiscreteInputs is read Input Registers data in ModBus Server & send response
func (server *MBServer) readInputRegisters(r *mbResponse, startAddress, quantity uint16) Exception {
	register := make([]byte, 2)
	buff := []byte{}
	var i uint16
	server.Devices[r.UnitID].RWInputRegisters.RLock()
	defer server.Devices[r.UnitID].RWInputRegisters.RUnlock()
	for i = startAddress; i < (startAddress + quantity); i++ {
		if _, ok := server.Devices[r.UnitID]; !ok {
			return IllegalDataAddress
		}
		if _, ok := server.Devices[r.UnitID].InputRegisters[i]; !ok {
			return IllegalDataAddress
		}
		binary.BigEndian.PutUint16(register, server.Devices[r.UnitID].InputRegisters[i])
		buff = append(buff, register...)
	}
	r.Data = append(r.Data, byte(len(buff)))
	r.Data = append(r.Data, buff...)
	return Success
}

func (server *MBServer) WriteCoils(unitid UnitID, address uint16, value bool) {
	server.Devices[unitid].RWCoils.Lock()
	defer server.Devices[unitid].RWCoils.Unlock()
	server.Devices[unitid].Coils[address] = value
}
func (server *MBServer) WriteDiscreteInputs(unitid UnitID, address uint16, value bool) {
	server.Devices[unitid].RWDiscreteInputs.Lock()
	defer server.Devices[unitid].RWDiscreteInputs.Unlock()
	server.Devices[unitid].DiscreteInputs[address] = value
}
func (server *MBServer) WriteHoldingRegisters(unitid UnitID, address, value uint16) {
	server.Devices[unitid].RWHoldingRegisters.Lock()
	defer server.Devices[unitid].RWHoldingRegisters.Unlock()
	server.Devices[unitid].HoldingRegisters[address] = value
}
func (server *MBServer) WriteInputRegisters(unitid UnitID, address, value uint16) {
	server.Devices[unitid].RWInputRegisters.Lock()
	defer server.Devices[unitid].RWInputRegisters.Unlock()
	server.Devices[unitid].InputRegisters[address] = value
}

/*
func (server *ModbusServer) Shutdown(ctx context.Context) error {
	// srv.inShutdown.setTrue()

	// srv.mu.Lock()
	// lnerr := srv.closeListenersLocked()
	// srv.closeDoneChanLocked()
	// for _, f := range srv.onShutdown {
	// 	go f()
	// }
	// srv.mu.Unlock()

	// pollIntervalBase := time.Millisecond
	// nextPollInterval := func() time.Duration {
	// 	// Add 10% jitter.
	// 	interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10)))
	// 	// Double and clamp for next time.
	// 	pollIntervalBase *= 2
	// 	if pollIntervalBase > shutdownPollIntervalMax {
	// 		pollIntervalBase = shutdownPollIntervalMax
	// 	}
	// 	return interval
	// }

	// timer := time.NewTimer(nextPollInterval())
	// defer timer.Stop()
	// for {
	// 	if srv.closeIdleConns() && srv.numListeners() == 0 {
	// 		return lnerr
	// 	}
	// 	select {
	// 	case <-ctx.Done():
	// 		return ctx.Err()
	// 	case <-timer.C:
	// 		timer.Reset(nextPollInterval())
	// 	}
	// }
	return nil
}

func (server *ModbusServer) closeListenersLocked() error {
	// var err error
	// for ln := range s.listeners {
	// 	if cerr := (*ln).Close(); cerr != nil && err == nil {
	// 		err = cerr
	// 	}
	// }
	return nil
}
*/
