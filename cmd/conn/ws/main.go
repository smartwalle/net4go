package main

import (
	"fmt"
	websocket "github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/cmd/conn/protocol"
	"github.com/smartwalle/net4go/ws"
	"time"
)

func main() {
	var p = &protocol.WSProtocol{}
	var h = &WSHandler{}

	for i := 0; i < 1000; i++ {
		c, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:6656/ws", nil)
		if err != nil {
			fmt.Println(err)
			return
		}

		var nSess = ws.NewSession(c, ws.Text, p, h)

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 WS"

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

type WSHandler struct {
}

func (this *WSHandler) OnMessage(sess net4go.Session, packet net4go.Packet) {
	//fmt.Println("OnMessage", packet)
}

func (this *WSHandler) OnClose(sess net4go.Session, err error) {
	fmt.Println("OnClose", err)
}
