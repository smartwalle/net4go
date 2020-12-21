package main

import (
	"fmt"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/cmd/conn/protocol"
	"net"
	"time"
)

func main() {
	var p = &protocol.TCPProtocol{}
	var h = &TCPHandler{}

	for i := 0; i < 1000; i++ {
		c, err := net.Dial("tcp", ":9999")
		if err != nil {
			fmt.Println(err)
			return
		}

		var nConn = net4go.NewConn(c, p, h, net4go.WithNoDelay(false), net4go.WithReadTimeout(time.Second*15), net4go.WithWriteTimeout(time.Second*10))

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 TCP"

		go func(i int, nConn net4go.Conn) {
			fmt.Println("--", i)
			for {
				nConn.WritePacket(packet)
				time.Sleep(time.Millisecond * 66)
			}
		}(i, nConn)
	}

	select {}
}

type TCPHandler struct {
}

func (this *TCPHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	//fmt.Println("OnMessage", packet)
	return true
}

func (this *TCPHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
