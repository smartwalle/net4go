package main

import (
	"fmt"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/sample/conn/protocol"
	"net"
	"time"
)

func main() {
	var p = &protocol.TCPProtocol{}
	var h = &TCPHandler{}

	c, err := net.Dial("tcp", ":6655")
	if err != nil {
		fmt.Println(err)
		return
	}

	var nConn = net4go.NewConn(c, p, h)

	var packet = &protocol.Packet{}
	packet.Type = 1
	packet.Message = "来自 TCP"

	for {
		nConn.WritePacket(packet)
		time.Sleep(time.Second * 1)
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
}
