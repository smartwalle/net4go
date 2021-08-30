package quic

import (
	"context"
	"crypto/tls"
	"github.com/lucas-clemente/quic-go"
	"net"
)

type Listener struct {
	ln   quic.Listener
	conn net.PacketConn
}

func (this *Listener) PacketConn() net.PacketConn {
	return this.conn
}

func (this *Listener) Close() error {
	return this.ln.Close()
}

func (this *Listener) Addr() net.Addr {
	return this.ln.Addr()
}

func (this *Listener) Accept() (net.Conn, error) {
	sess, err := this.ln.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := sess.AcceptStream(context.Background())
	if err != nil {
		sess.CloseWithError(quic.ApplicationErrorCode(0), err.Error())
		return nil, err
	}
	c := &Conn{sess: sess, Stream: stream}
	return c, nil
}

func Listen(addr string, tlsConf *tls.Config, config *quic.Config) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	return ListenPacket(conn, tlsConf, config)
}

func ListenPacket(conn net.PacketConn, tlsConf *tls.Config, config *quic.Config) (*Listener, error) {
	l, err := quic.Listen(conn, tlsConf, config)
	if err != nil {
		return nil, err
	}

	ln := &Listener{ln: l, conn: conn}
	return ln, nil
}
