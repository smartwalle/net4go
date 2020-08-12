package main

import (
	"fmt"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/cmd/conn/protocol"
	"net"
	"os"
	"time"
)

func main() {
	var p = &protocol.TCPProtocol{}
	var h = &TCPHandler{}

	for i := 0; i < 1000; i++ {
		c, err := net.Dial("tcp", ":6655")
		if err != nil {
			fmt.Println(err)
			return
		}

		var nConn = net4go.NewConn(c, p, h)

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 TCP"

		go func(nConn net4go.Conn) {
			for {
				nConn.WritePacket(packet)
				time.Sleep(time.Millisecond * 10)
			}
		}(nConn)
	}

	select {}
}

type TCPHandler struct {
}

func (this *TCPHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	fmt.Println("OnMessage", packet)
	return true
}

func (this *TCPHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
	os.Exit(-1)
}
