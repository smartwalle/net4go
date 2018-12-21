package net4go

import "net"

// --------------------------------------------------------------------------------
type Conn interface {
	Identifier() string

	Tag() string

	SetHandler(h ConnHandler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	Close() error

	LocalAddr() net.Addr

	RemoteAddr() net.Addr

	Write(data []byte)
}

// --------------------------------------------------------------------------------
type ConnHandler interface {
	DidOpenConn(s Conn)

	DidClosedConn(s Conn)

	DidWrittenData(s Conn, data []byte)

	DidReceivedData(s Conn, data []byte)
}

// --------------------------------------------------------------------------------
type ConnHub interface {
	AddConn(Conn)
}

// --------------------------------------------------------------------------------
type hub struct {
}
