package net4go

import "encoding/binary"

type Packet interface {
	Marshal() ([]byte, error)

	Unmarshal([]byte) error
}

type DefaultPacket struct {
	pType uint16
	data  []byte
}

func (this *DefaultPacket) Marshal() ([]byte, error) {
	var data = make([]byte, 2+len(this.data))
	binary.BigEndian.PutUint16(data[0:2], this.pType)
	copy(data[2:], this.data)
	return data, nil
}

func (this *DefaultPacket) Unmarshal(data []byte) error {
	this.pType = binary.BigEndian.Uint16(data[:2])
	this.data = data[2:]
	return nil
}

func (this *DefaultPacket) GetData() []byte {
	return this.data
}

func (this *DefaultPacket) GetType() uint16 {
	return this.pType
}

func NewDefaultPacket(pType uint16, data []byte) *DefaultPacket {
	var p = &DefaultPacket{}
	p.pType = pType
	p.data = data
	return p
}
