package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/net4go/cmd/conn/protocol"
	"github.com/smartwalle/net4go/quic"
	"github.com/smartwalle/net4go/ws"
	"math/big"
	"net"
	"net/http"
)

func main() {
	var h = &ServerHandler{}

	go serveTcp(h)
	go serveWs(h)
	go serveQUIC(h)

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
		var nc = ws.NewConn(c, ws.Text, p, h)

		var p = &protocol.Packet{}
		p.Type = 2
		p.Message = "这是服务器端回复的消息"
		nc.AsyncWritePacket(p)

	})
	http.ListenAndServe(":6656", nil)
}

func serveQUIC(h net4go.Handler) {
	l, err := quic.Listen("127.0.0.1:6657", generateTLSConfig(), nil)
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

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}

type ServerHandler struct {
}

func (this *ServerHandler) OnMessage(conn net4go.Conn, packet net4go.Packet) bool {
	//fmt.Println("OnMessage", packet)

	var p = &protocol.Packet{}
	p.Type = 2
	p.Message = "这是服务器端回复的消息"
	conn.AsyncWritePacket(p)

	return true
}

func (this *ServerHandler) OnClose(conn net4go.Conn, err error) {
	fmt.Println("OnClose", err)
}
