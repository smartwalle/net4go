package net4go

import "encoding/binary"

type Packet interface {
	Serialize() []byte
}

type DefaultPacket struct {
	data []byte
}

func (this *DefaultPacket) Serialize() []byte {
	var data = make([]byte, 4+len(this.data))
	binary.BigEndian.PutUint32(data[0:4], uint32(len(this.data)))
	copy(data[4:], this.data)
	return data
}

func (this *DefaultPacket) GetData() []byte {
	return this.data
}

func NewDefaultPacket(data []byte) *DefaultPacket {
	var p = &DefaultPacket{}
	p.data = data
	return p
}
