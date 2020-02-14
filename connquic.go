package net4go

import (
	"context"
	"crypto/tls"
	"errors"
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

	if stream == nil {
		sess.Close()
		return nil, errors.New("closed stream")
	}

	c := &qSession{sess: sess, Stream: stream}
	return c, nil
}

func DialQUIC(addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d QUICDialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialContext(context.Background(), "udp", addr)
}

func DialQUICWithConn(pConn net.PacketConn, addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d QUICDialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialConnContext(context.Background(), pConn, addr)
}

type QUICListener struct {
	ln              quic.Listener
	acceptConn      chan *qConn
	ReadBufferSize  int
	WriteBufferSize int
}

func (this *QUICListener) doAccept() {
	for {
		sess, err := this.ln.Accept(context.Background())
		if err != nil {
			return
		}

		go func(sess quic.Session) {
			for {
				stream, err := sess.AcceptStream(context.Background())
				if err != nil {
					sess.Close()
					return
				}

				if stream == nil {
					sess.Close()
					return
				}

				this.acceptConn <- &qConn{
					conn: &qSession{sess: sess, Stream: stream},
					err:  nil,
				}
			}
		}(sess)
	}
}

func (this *QUICListener) Accept() (net.Conn, error) {
	ac := <-this.acceptConn
	if ac.err != nil {
		return nil, ac.err
	}
	return ac.conn, nil
}

func ListenQUIC(addr string, tlsConf *tls.Config, config *quic.Config) (*QUICListener, error) {
	l, err := quic.ListenAddr(addr, tlsConf, config)
	if err != nil {
		return nil, err
	}

	ln := &QUICListener{ln: l, acceptConn: make(chan *qConn, 1)}
	go ln.doAccept()
	return ln, nil
}

type qConn struct {
	conn net.Conn
	err  error
}

type qSession struct {
	sess quic.Session
	quic.Stream
}

func (this *qSession) LocalAddr() net.Addr {
	return this.sess.LocalAddr()
}

func (this *qSession) RemoteAddr() net.Addr {
	return this.sess.RemoteAddr()
}

func (this *qSession) Close() error {
	this.Stream.Close()
	return this.sess.Close()
}
