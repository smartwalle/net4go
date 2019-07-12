package net4go

type Handler interface {
	OnConnect(*Conn)

	OnMessage(*Conn, Packet) bool

	OnClose(*Conn, error)
}
