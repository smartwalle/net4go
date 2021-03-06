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

		var nConn = ws.NewConn(c, ws.Text, p, h)

		var packet = &protocol.Packet{}
		packet.Type = 1
		packet.Message = "来自 WS"

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

type WSHandler struct {
}

func (this *WSHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	//fmt.Println("OnMessage", packet)
	return true
}

func (this *WSHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
