package main

import (
	"fmt"
	"github.com/smartwalle/net4go"
	"net"
)

func main() {
	l, err := net.Listen("tcp", ":6655")
	if err != nil {
		fmt.Println(err)
		return
	}

	var p = &net4go.DefaultProtocol{}
	var h = &ServerHandler{}

	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}

		cc := net4go.NewConn(c, p, h)
		cc.WritePacket(net4go.NewDefaultPacket(1, []byte("welcome")))
	}
}

type ServerHandler struct {
}

func (this *ServerHandler) OnMessage(c *net4go.Conn, p net4go.Packet) bool {
	fmt.Println("OnMessage", p)

	switch v := p.(type) {
	case *net4go.DefaultPacket:
		fmt.Println(v.GetType(), string(v.GetData()))
		c.WritePacket(net4go.NewDefaultPacket(v.GetType(), []byte("Yes")))
	}
	return true
}

func (this *ServerHandler) OnClose(c *net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
