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
	"sync"
	"time"
)

var sessList = make(map[net4go.Session]time.Time)
var sessMu = &sync.Mutex{}

func main() {
	var h = &ServerHandler{}

	go serveTcp(h)
	go serveWs(h)
	go serveQUIC(h)

	select {}
}

func serveTcp(h net4go.Handler) {
	l, err := net.Listen("tcp", ":6555")
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

		var nSess = net4go.NewSession(c, p, h, net4go.WithNoDelay(false), net4go.WithReadTimeout(time.Second*15), net4go.WithWriteTimeout(time.Second*10))

		sessMu.Lock()
		sessList[nSess] = time.Now()
		sessMu.Unlock()
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
		var nSess = ws.NewSession(c, ws.Text, p, h)

		var p = &protocol.Packet{}
		p.Type = 2
		p.Message = "这是服务器端回复的消息"
		nSess.AsyncWritePacket(p)

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

		net4go.NewSession(c, p, h)
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

func (this *ServerHandler) OnMessage(sess net4go.Session, packet net4go.Packet) {
	fmt.Println("OnMessage", packet)

	var p = &protocol.Packet{}
	p.Type = 2
	p.Message = "这是服务器端回复的消息"
	sess.AsyncWritePacket(p)
}

func (this *ServerHandler) OnClose(sess net4go.Session, err error) {
	sessMu.Lock()
	var ct = sessList[sess]
	delete(sessList, sess)
	sessMu.Unlock()

	fmt.Println("OnClose", time.Now(), "  =====  ", ct, err)
}
