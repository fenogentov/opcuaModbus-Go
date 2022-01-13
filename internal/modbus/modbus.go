package modbus

import (
	"strings"
	"sync"
)

type Exception uint8 // exception response Modbus
type UnitID uint8    // id device Modbus

const (
	// coils
	ReadCoils          uint8 = 0x01
	WriteSingleCoil    uint8 = 0x05
	WriteMultipleCoils uint8 = 0x0f

	// discrete inputs
	ReadDiscreteInputs uint8 = 0x02

	// 16-bit input/holding registers
	ReadHoldingRegisters   uint8 = 0x03
	ReadInputRegisters     uint8 = 0x04
	WriteSingleRegister    uint8 = 0x06
	WriteMultipleRegisters uint8 = 0x10

	// exception codes
	Success            Exception = 0x00
	IllegalFunction    Exception = 0x01
	IllegalDataAddress Exception = 0x02
	IllegalDataValue   Exception = 0x03
	SlaveDeviceFailure Exception = 0x04
)

// MBData is device ModBus registers data storage
type MBData struct {
	RWCoils            *sync.RWMutex
	RWDiscreteInputs   *sync.RWMutex
	RWHoldingRegisters *sync.RWMutex
	RWInputRegisters   *sync.RWMutex
	Coils              map[uint16]bool
	DiscreteInputs     map[uint16]bool
	HoldingRegisters   map[uint16]uint16
	InputRegisters     map[uint16]uint16
}

// ModbusResponse is structure for sending a Modbus response
type mbResponse struct {
	transactionID uint16
	protocolID    uint16
	lenght        uint16
	UnitID        UnitID
	function      uint8
	Data          []byte
}

// StringToUint8 is converting name function ModBus to numeric uint8
func StringToUint8(s string) uint8 {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "coil"), s == "1":
		return 1
	case strings.Contains(s, "discret"), s == "2":
		return 2
	case strings.Contains(s, "holding"), s == "3":
		return 3
	case strings.Contains(s, "input"), s == "4":
		return 4
	default:
		return 0
	}
}

func (excp Exception) String() string {
	switch excp {
	case 0x00:
		return "Success"
	case 0x01:
		return "IllegalFunction"
	case 0x02:
		return "IllegalDataAddress"
	case 0x03:
		return "IllegalDataValue"
	case 0x04:
		return "SlaveDeviceFailure"
	default:
		return ""
	}
}
