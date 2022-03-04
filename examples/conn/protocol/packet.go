package protocol

import "encoding/binary"

type Packet struct {
	Type    uint16 `json:"type"`
	Message string `json:"message"`
}

func (this *Packet) MarshalPacket() ([]byte, error) {
	var msg = []byte(this.Message)
	var data = make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(data[0:2], this.Type)
	copy(data[2:], msg)
	return data, nil
}

func (this *Packet) UnmarshalPacket(data []byte) error {
	this.Type = binary.BigEndian.Uint16(data[:2])
	this.Message = string(data[2:])
	return nil
}
