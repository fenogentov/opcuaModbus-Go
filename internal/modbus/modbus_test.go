package modbus

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"opcuaModbus/internal/logger"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

var mbPort = 1508
var filelogg = "logtst.log"

var tNameFuncMB = []struct {
	in     string
	expect uint8
	msgErr string
}{
	{"coil", 1, "function does not work "},
	{"discrete", 2, "function does not work "},
	{"holding", 3, "function does not work "},
	{"input", 4, "function does not work "},
	{"coil  ", 1, "extra spaces are not processed"},
	{"  coil", 1, "extra spaces are not processed"},
	{"  coil  ", 1, "extra spaces are not processed"},
	{"COIL", 1, "lowercase letters are not processed "},
	{"registers", 0, "function does not work "},
}

type ReadModbus struct {
	request     []byte
	want        []byte
	description string
}

var tReadMBok = []ReadModbus{
	{request: []byte{0, 1, 0, 0, 0, 6, 1, 1, 0, 100, 0, 1},
		want:        []byte{0, 1, 0, 0, 0, 4, 1, 1, 1, 1},
		description: "read single Coil",
	},
	{request: []byte{0, 2, 0, 0, 0, 6, 1, 1, 0, 101, 0, 5},
		want:        []byte{0, 2, 0, 0, 0, 4, 1, 1, 1, 31},
		description: "read multiple Coil",
	},
	{request: []byte{0, 3, 0, 0, 0, 6, 2, 2, 0, 200, 0, 1},
		want:        []byte{0, 3, 0, 0, 0, 4, 2, 2, 1, 1},
		description: "read single Discrete inputs",
	},
	{request: []byte{0, 4, 0, 0, 0, 1, 2, 2, 0, 201, 0, 5},
		want:        []byte{0, 4, 0, 0, 0, 4, 2, 2, 1, 31},
		description: "read multiple Discrete inputs",
	},
	{request: []byte{0, 5, 0, 0, 0, 6, 3, 3, 0, 100, 0, 1},
		want:        []byte{0, 5, 0, 0, 0, 5, 3, 3, 2, 0, 111},
		description: "read single Holding registers",
	},
	{request: []byte{0, 6, 0, 0, 0, 6, 3, 3, 0, 101, 0, 5},
		want:        []byte{0, 6, 0, 0, 0, 13, 3, 3, 10, 0, 222, 1, 77, 1, 188, 2, 43, 2, 154},
		description: "read multiple Holding registers",
	},
	{request: []byte{0, 7, 0, 0, 0, 6, 4, 4, 0, 200, 0, 1},
		want:        []byte{0, 7, 0, 0, 0, 5, 4, 4, 2, 4, 87},
		description: "read single Input registers",
	},
	{request: []byte{0, 8, 0, 0, 0, 6, 4, 4, 0, 201, 0, 5},
		want:        []byte{0, 8, 0, 0, 0, 13, 4, 4, 10, 8, 174, 13, 5, 17, 92, 21, 179, 26, 10},
		description: "read multiple Input registers",
	},
}

var tReadMBexcept = []struct {
	request     []byte
	want        []byte
	err         error
	description string
}{
	{request: []byte{1, 2, 3, 4, 5},
		want:        []byte{},
		err:         io.EOF,
		description: "small request packet",
	},
	{request: []byte{0, 11, 0, 0, 0, 6, 248, 4, 0, 201, 0, 5},
		want:        []byte{0, 11, 0, 0, 0, 3, 248, 132, 4},
		err:         nil,
		description: "SlaveDeviceFailure (unitid >247)",
	},
	{request: []byte{0, 12, 0, 0, 0, 6, 11, 4, 0, 201, 0, 5},
		want:        []byte{0, 12, 0, 0, 0, 3, 11, 132, 4},
		err:         nil,
		description: "SlaveDeviceFailure (non-existent unitid)",
	},
	{request: []byte{0, 13, 0, 0, 0, 6, 1, 1, 0, 100, 0, 0},
		want:        []byte{0, 13, 0, 0, 0, 3, 1, 129, 3},
		err:         nil,
		description: "IllegalDataValue (coil quantity <1)",
	},
	{request: []byte{0, 14, 0, 0, 0, 6, 1, 1, 0, 100, 7, 209},
		want:        []byte{0, 14, 0, 0, 0, 3, 1, 129, 3},
		err:         nil,
		description: "IllegalDataValue (coil quantity >2000)",
	},
	{request: []byte{0, 15, 0, 0, 0, 6, 1, 1, 78, 32, 177, 244},
		want:        []byte{0, 15, 0, 0, 0, 3, 1, 129, 3},
		err:         nil,
		description: "IllegalDataValue (coil startingAddress+quantity > 65535)",
	},
	{request: []byte{0, 16, 0, 0, 0, 6, 2, 2, 0, 200, 0, 0},
		want:        []byte{0, 16, 0, 0, 0, 3, 2, 130, 3},
		err:         nil,
		description: "IllegalDataValue (discrete input quantity <1)",
	},
	{request: []byte{0, 17, 0, 0, 0, 6, 2, 2, 0, 200, 7, 209},
		want:        []byte{0, 17, 0, 0, 0, 3, 2, 130, 3},
		err:         nil,
		description: "IllegalDataValue (discrete input quantity >2000)",
	},
	{request: []byte{0, 18, 0, 0, 0, 6, 2, 2, 78, 32, 177, 244},
		want:        []byte{0, 18, 0, 0, 0, 3, 2, 130, 3},
		err:         nil,
		description: "IllegalDataValue (discrete input startingAddress+quantity > 65535)",
	},

	{request: []byte{0, 19, 0, 0, 0, 6, 3, 3, 0, 200, 0, 0},
		want:        []byte{0, 19, 0, 0, 0, 3, 3, 131, 3},
		err:         nil,
		description: "IllegalDataValue (holding register quantity <1)",
	},
	{request: []byte{0, 20, 0, 0, 0, 6, 3, 3, 0, 200, 7, 209},
		want:        []byte{0, 20, 0, 0, 0, 3, 3, 131, 3},
		err:         nil,
		description: "IllegalDataValue (holding register quantity >2000)",
	},
	{request: []byte{0, 21, 0, 0, 0, 6, 3, 3, 78, 32, 177, 244},
		want:        []byte{0, 21, 0, 0, 0, 3, 3, 131, 3},
		err:         nil,
		description: "IllegalDataValue (holding register startingAddress+quantity > 65535)",
	},

	{request: []byte{0, 22, 0, 0, 0, 6, 4, 4, 0, 200, 0, 0},
		want:        []byte{0, 22, 0, 0, 0, 3, 4, 132, 3},
		err:         nil,
		description: "IllegalDataValue (input register quantity <1)",
	},
	{request: []byte{0, 23, 0, 0, 0, 6, 4, 4, 0, 200, 7, 209},
		want:        []byte{0, 23, 0, 0, 0, 3, 4, 132, 3},
		err:         nil,
		description: "IllegalDataValue (input register quantity >2000)",
	},
	{request: []byte{0, 24, 0, 0, 0, 6, 4, 4, 78, 32, 177, 244},
		want:        []byte{0, 24, 0, 0, 0, 3, 4, 132, 3},
		err:         nil,
		description: "IllegalDataValue (input register startingAddress+quantity > 65535)",
	},
}

func TestStringToUint8(t *testing.T) {
	for _, el := range tNameFuncMB {
		out := StringToUint8(el.in)
		if out != el.expect {
			t.Errorf("%v. Input: \"%v\", want: %v, got: %v", el.msgErr, el.in, el.expect, out)
		}
	}
}

func TestMBServer(t *testing.T) {

	var logg = logger.New(filelogg, "debug")
	defer os.Remove(filelogg)
	var mbserver = NewServer(logg, "", mbPort)

	t.Run("AddDevices", func(t *testing.T) {
		mbserver.AddDevice(1)
		mbserver.AddDevice(2)
		mbserver.AddDevice(3)
		mbserver.AddDevice(4)
		if len(mbserver.Devices) != 4 {
			t.Errorf("error adding a device to the ModBus Server structure")
		}
	})
	t.Run("Listen", func(t *testing.T) {
		go mbserver.Listen()
		t1 := time.NewTimer(50 * time.Millisecond)
		<-t1.C
		prt := strconv.Itoa(mbPort)
		client, err := net.Dial("tcp", "127.0.0.1:"+prt)
		if err != nil {
			t.Error("failed connect to Test ModBus Server: ", err)
		}
		defer client.Close()
	})
	t.Run("RWCoils", func(t *testing.T) {
		mbserver.WriteCoils(1, 100, true)
		mbserver.WriteCoils(1, 101, true)
		mbserver.WriteCoils(1, 102, true)
		mbserver.WriteCoils(1, 103, true)
		mbserver.WriteCoils(1, 104, true)
		mbserver.WriteCoils(1, 105, true)
		if mbserver.Devices[1].Coils[100] != true || mbserver.Devices[1].Coils[105] != true {
			t.Error("error write Coils ModBus Server")
		}
	})
	t.Run("RWDiscreteInputs", func(t *testing.T) {
		mbserver.WriteDiscreteInputs(2, 200, true)
		mbserver.WriteDiscreteInputs(2, 201, true)
		mbserver.WriteDiscreteInputs(2, 202, true)
		mbserver.WriteDiscreteInputs(2, 203, true)
		mbserver.WriteDiscreteInputs(2, 204, true)
		mbserver.WriteDiscreteInputs(2, 205, true)
		if mbserver.Devices[2].DiscreteInputs[200] != true || mbserver.Devices[2].DiscreteInputs[205] != true {
			t.Error("error write Discrete inputs ModBus Server")
		}
	})
	t.Run("RWHoldingRegisters", func(t *testing.T) {
		mbserver.WriteHoldingRegisters(3, 100, 111)
		mbserver.WriteHoldingRegisters(3, 101, 222)
		mbserver.WriteHoldingRegisters(3, 102, 333)
		mbserver.WriteHoldingRegisters(3, 103, 444)
		mbserver.WriteHoldingRegisters(3, 104, 555)
		mbserver.WriteHoldingRegisters(3, 105, 666)
		if mbserver.Devices[3].HoldingRegisters[100] != 111 || mbserver.Devices[3].HoldingRegisters[105] != 666 {
			t.Error("error write Holding registers ModBus Server")
		}
	})
	t.Run("RWInputRegisters", func(t *testing.T) {
		mbserver.WriteInputRegisters(4, 200, 1111)
		mbserver.WriteInputRegisters(4, 201, 2222)
		mbserver.WriteInputRegisters(4, 202, 3333)
		mbserver.WriteInputRegisters(4, 203, 4444)
		mbserver.WriteInputRegisters(4, 204, 5555)
		mbserver.WriteInputRegisters(4, 205, 6666)
		if mbserver.Devices[4].InputRegisters[200] != 1111 || mbserver.Devices[4].InputRegisters[205] != 6666 {
			t.Error("error write Input registers ModBus Server")
		}
	})
	t.Run("ModbusOK", func(t *testing.T) {
		var wg sync.WaitGroup
		for _, mb := range tReadMBok {
			wg.Add(1)
			go func(mb ReadModbus) {
				prt := strconv.Itoa(mbPort)
				client, err := net.Dial("tcp", "127.0.0.1:"+prt)
				t1 := time.NewTimer(50 * time.Millisecond)
				<-t1.C
				if err != nil {
					t.Error("failed connect to Test ModBus Server: ", err)
				} else {
					defer client.Close()
				}

				defer wg.Done()
				if _, err := client.Write(mb.request); err != nil {
					t.Error("could not request to TCP server:", err)
				}
				buf := make([]byte, 1024)
				b, err := client.Read(buf)
				buf = buf[:b]
				if err != nil {
					fmt.Println("Error reading:", err.Error())
				}
				if !bytes.Equal(buf, mb.want) {
					t.Errorf("error %s | got: %v, want: %v", mb.description, buf, mb.want)
				}
			}(mb)
		}
		wg.Wait()
	})
	t.Run("ModbusException", func(t *testing.T) {

		for _, mb := range tReadMBexcept {
			prt := strconv.Itoa(mbPort)
			client, err := net.Dial("tcp", "127.0.0.1:"+prt)
			t1 := time.NewTimer(50 * time.Millisecond)
			<-t1.C
			if err != nil {
				t.Error("failed connect to Test ModBus Server: ", err)
			} else {
				defer client.Close()
			}
			if _, err := client.Write(mb.request); err != nil {
				t.Error("could not request to TCP server:", err)
			}
			buf := make([]byte, 1024)
			b, err := client.Read(buf)
			buf = buf[:b]
			if err != nil {
				if err != mb.err {
					fmt.Println("Error reading:", err.Error())
				}
			}
			if !bytes.Equal(buf, mb.want) {
				t.Errorf("error %s | got: %v, want: %v", mb.description, buf, mb.want)
			}
		}
	})
}
