package main

import (
	"crypto/tls"
	"fmt"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/examples/conn/protocol"
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

		var nSess = net4go.NewSession(c, p, h)

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 QUIC"

		go func(i int, nSess net4go.Session) {
			fmt.Println("--", i)
			for {
				nSess.AsyncWritePacket(packet)
				time.Sleep(time.Millisecond * 10)
			}
		}(i, nSess)
	}

	select {}
}

type QUICHandler struct {
}

func (this *QUICHandler) OnMessage(sess net4go.Session, packet net4go.Packet) {
	//fmt.Println("OnMessage", packet)
}

func (this *QUICHandler) OnClose(sess net4go.Session, err error) {
	fmt.Println("OnClose", err)
}
