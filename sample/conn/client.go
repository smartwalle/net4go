package main

import (
	"fmt"
	"github.com/smartwalle/net4go"
	"net"
	"time"
)

func main() {
	c, err := net.Dial("tcp", ":6655")
	if err != nil {
		fmt.Println(err)
		return
	}

	var p = &net4go.DefaultProtocol{}
	var h = &ClientHandler{}

	cc := net4go.NewConn(c, p, h)

	for i := 0; i < 1000; i++ {
		cc.WritePacket(net4go.NewDefaultPacket(uint16(i), []byte("hello")))
		time.Sleep(time.Second * 1)
	}
}

type ClientHandler struct {
}

func (this *ClientHandler) OnMessage(c net4go.Conn, p net4go.Packet) bool {
	fmt.Println("OnMessage", p)

	switch v := p.(type) {
	case *net4go.DefaultPacket:
		fmt.Println(v.GetType(), string(v.GetData()))
	}
	return true
}

func (this *ClientHandler) OnClose(c net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
