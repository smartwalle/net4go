package net4go

import (
	"encoding/binary"
	"io"
)

type Protocol interface {
	ReadPacket(r io.Reader) (Packet, error)
}

type DefaultProtocol struct {
}

func (this *DefaultProtocol) ReadPacket(r io.Reader) (Packet, error) {
	var lenBytes = make([]byte, 4)
	if _, err := io.ReadFull(r, lenBytes); err != nil {
		return nil, err
	}

	var length = binary.BigEndian.Uint32(lenBytes)
	var buff = make([]byte, length)

	if _, err := io.ReadFull(r, buff); err != nil {
		return nil, err
	}
	return NewDefaultPacket(buff), nil
}
