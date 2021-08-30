package quic

import (
	"context"
	"crypto/tls"
	"github.com/lucas-clemente/quic-go"
	"net"
)

type Dialer struct {
	tlsConf *tls.Config
	config  *quic.Config
}

func NewDialer(tlsConf *tls.Config, config *quic.Config) *Dialer {
	var d = &Dialer{}
	d.tlsConf = tlsConf
	d.config = config
	return d
}

func (this *Dialer) Dial(network, addr string) (net.Conn, error) {
	return this.DialContext(context.Background(), network, addr)
}

func (this *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	udpConn, err := net.ListenUDP(network, &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	return this.DialConnContext(ctx, udpConn, addr)
}

func (this *Dialer) DialConnContext(ctx context.Context, pConn net.PacketConn, addr string) (net.Conn, error) {
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
		sess.CloseWithError(quic.ApplicationErrorCode(0), err.Error())
		return nil, err
	}

	c := &Conn{sess: sess, Stream: stream}
	return c, nil
}

func Dial(addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d Dialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialContext(context.Background(), "udp", addr)
}

func DialPacket(pConn net.PacketConn, addr string, tlsConf *tls.Config, config *quic.Config) (net.Conn, error) {
	var d Dialer
	d.tlsConf = tlsConf
	d.config = config
	return d.DialConnContext(context.Background(), pConn, addr)
}
