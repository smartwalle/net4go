package quic

import (
	"github.com/lucas-clemente/quic-go"
	"net"
)

type Conn struct {
	sess quic.Connection
	quic.Stream
}

func (this *Conn) LocalAddr() net.Addr {
	return this.sess.LocalAddr()
}

func (this *Conn) RemoteAddr() net.Addr {
	return this.sess.RemoteAddr()
}

func (this *Conn) Close() error {
	this.Stream.Close()
	return this.sess.CloseWithError(quic.ApplicationErrorCode(0), "")
}
