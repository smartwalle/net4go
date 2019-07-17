package net4go

import (
	"encoding/binary"
	"io"
)

type Protocol interface {
	// Marshal 把满足 Packet 接口的对象转换为 []byte
	Marshal(p Packet) []byte

	// Unmarshal 从 io.Reader 读取数据，转换为相应的满足 Packet 接口的对象
	// 具体的转换规则需要由开发者自己实现
	Unmarshal(r io.Reader) (Packet, error)
}

type DefaultProtocol struct {
}

func (this *DefaultProtocol) Marshal(p Packet) []byte {
	var pData = p.Marshal()
	var data = make([]byte, 4+len(pData))
	binary.BigEndian.PutUint32(data[0:4], uint32(len(pData)))
	copy(data[4:], pData)
	return data
}

func (this *DefaultProtocol) Unmarshal(r io.Reader) (Packet, error) {
	var lengthBytes = make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBytes); err != nil {
		return nil, err
	}
	var length = binary.BigEndian.Uint32(lengthBytes)

	var buff = make([]byte, length)
	if _, err := io.ReadFull(r, buff); err != nil {
		return nil, err
	}

	var p = &DefaultPacket{}
	if err := p.Unmarshal(buff); err != nil {
		return nil, err
	}
	return p, nil
}
