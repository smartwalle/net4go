package net4go

import (
	"context"
	"crypto/tls"
	"github.com/lucas-clemente/quic-go"
	"net"
)

type QUICDialer struct {
	tlsConf *tls.Config
	config  *quic.Config
}

func NewQUICDialer(tlsConf *tls.Config, config *quic.Config) *QUICDialer {
	var d = &QUICDialer{}
	d.tlsConf = tlsConf
	d.config = config
	return d
}

func (this *QUICDialer) Dial(network, addr string) (net.Conn, error) {
	return this.DialContext(context.Background(), network, addr)
}

func (this *QUICDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	udpConn, err := net.ListenUDP(network, &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	return this.DialConnContext(ctx, udpConn, addr)
}

func (this *QUICDialer) DialConnContext(ctx context.Context, pConn net.PacketConn, addr string) (net.Conn, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	sess, err := quic.DialContext(ctx, pConn, raddr, addr, this.tlsConf, this.config)
	if err != nil {
		return nil, err
	}

	stream, err := sess.OpenStream()
	if err != nil {
		sess.Close()
		return nil, err
	}

	c := &QUICConn{sess: sess, Stream: stream}
	return c, nil
}

func DialQUICWithAddr(addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d QUICDialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialContext(context.Background(), "udp", addr)
}

func DialQUIC(pConn net.PacketConn, addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d QUICDialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialConnContext(context.Background(), pConn, addr)
}

type QUICListener struct {
	ln   quic.Listener
	conn net.PacketConn
}

func (this *QUICListener) PacketConn() net.PacketConn {
	return this.conn
}

func (this *QUICListener) Close() error {
	return this.ln.Close()
}

func (this *QUICListener) Addr() net.Addr {
	return this.ln.Addr()
}

func (this *QUICListener) Accept() (net.Conn, error) {
	sess, err := this.ln.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := sess.AcceptStream(context.Background())
	if err != nil {
		sess.Close()
		return nil, err
	}
	c := &QUICConn{sess: sess, Stream: stream}
	return c, nil
}

func ListenQUICWithAddr(addr string, tlsConf *tls.Config, config *quic.Config) (*QUICListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	return ListenQUIC(conn, tlsConf, config)
}

func ListenQUIC(conn net.PacketConn, tlsConf *tls.Config, config *quic.Config) (*QUICListener, error) {
	l, err := quic.Listen(conn, tlsConf, config)
	if err != nil {
		return nil, err
	}

	ln := &QUICListener{ln: l, conn: conn}
	return ln, nil
}

type QUICConn struct {
	sess quic.Session
	quic.Stream
}

func (this *QUICConn) LocalAddr() net.Addr {
	return this.sess.LocalAddr()
}

func (this *QUICConn) RemoteAddr() net.Addr {
	return this.sess.RemoteAddr()
}

func (this *QUICConn) Close() error {
	this.Stream.Close()
	return this.sess.Close()
}
