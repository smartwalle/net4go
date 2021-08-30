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

	for i := 0; i < 1; i++ {
		c, err := net.Dial("tcp", ":6555")
		if err != nil {
			fmt.Println(err)
			return
		}

		var nSess = net4go.NewSession(c, p, h, net4go.WithNoDelay(false), net4go.WithReadTimeout(time.Second*15), net4go.WithWriteTimeout(time.Second*10))

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 TCP"

		go func(i int, nSess net4go.Session) {
			fmt.Println("--", i)
			for {
				nSess.WritePacket(packet)
			}
		}(i, nSess)
	}

	select {}
}

type TCPHandler struct {
}

func (this *TCPHandler) OnMessage(sess net4go.Session, packet net4go.Packet) bool {
	//fmt.Println("OnMessage", packet)
	return true
}

func (this *TCPHandler) OnClose(sess net4go.Session, err error) {
	fmt.Println("OnClose", err)
}
