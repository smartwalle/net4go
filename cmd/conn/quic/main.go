package main

import (
	"crypto/tls"
	"fmt"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/cmd/conn/protocol"
	"github.com/smartwalle/net4go/quic"
	"time"
)

func main() {
	var p = &protocol.TCPProtocol{}
	var h = &QUICHandler{}

	for i := 0; i < 1000; i++ {
		c, err := quic.Dial("127.0.0.1:6657", &tls.Config{InsecureSkipVerify: true,
			NextProtos: []string{"quic-echo-example"}}, nil)
		if err != nil {
			fmt.Println(err)
			return
		}

		var nConn = net4go.NewConn(c, p, h)

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 QUIC"

		go func(i int, nConn net4go.Conn) {
			fmt.Println("--", i)
			for {
				nConn.AsyncWritePacket(packet)
				time.Sleep(time.Millisecond * 10)
			}
		}(i, nConn)
	}

	select {}
}

type QUICHandler struct {
}

func (this *QUICHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	//fmt.Println("OnMessage", packet)
	return true
}

func (this *QUICHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
