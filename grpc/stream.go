package grpc

import "github.com/smartwalle/net4go"

type Stream interface {
	SendPacket(net4go.Packet) error

	RecvPacket() (net4go.Packet, error)

	OnClose(error)
}
