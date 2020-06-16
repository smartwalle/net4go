package main

import (
	"flag"
	"fmt"
	"github.com/smartwalle/net4go"
	"net"
	"time"
)

var pipe = net4go.NewPipeline(1024, time.Second*10, time.Second*10)

func main() {
	var proxy = flag.String("proxy", "ip:port", "代理地址")
	var listen = flag.String("listen", "0.0.0.0:6655", "监听地址")
	flag.Parse()

	l, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Println(err)
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go run(conn, *proxy)
	}
}

func run(conn net.Conn, to string) {
	nConn, err := net.Dial("tcp", to)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer nConn.Close()
	defer conn.Close()

	if err = pipe.Bind(conn, nConn); err != nil {
		fmt.Println(err)
		return
	}
}
