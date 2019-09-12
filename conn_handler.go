package net4go

type Handler interface {
	OnMessage(Conn, Packet) bool

	OnClose(Conn, error)
}
