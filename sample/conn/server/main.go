package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/sample/conn/protocol"
	"net"
	"net/http"
)

func main() {
	var h = &ServerHandler{}

	go serveTcp(h)
	go serveWs(h)

	select {}
}

func serveTcp(h net4go.Handler) {
	l, err := net.Listen("tcp", ":6655")
	if err != nil {
		fmt.Println(err)
		return
	}

	var p = &protocol.TCPProtocol{}

	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}

		net4go.NewConn(c, p, h)
	}
}

func serveWs(h net4go.Handler) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}

	var p = &protocol.WSProtocol{}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		var c, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		net4go.NewWsConn(c, p, h)
	})
	http.ListenAndServe(":6656", nil)
}

type ServerHandler struct {
}

func (this *ServerHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	fmt.Println("OnMessage", packet)

	var p = &protocol.Packet{}
	p.Type = 2
	p.Message = "这是服务器端回复的消息"
	conn.WritePacket(p)

	return true
}

func (this *ServerHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
