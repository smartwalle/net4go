package protocol

import (
	"encoding/binary"
	"encoding/json"
	"github.com/smartwalle/net4go"
	"io"
)

type TCPProtocol struct {
}

func (this *TCPProtocol) Marshal(p net4go.Packet) ([]byte, error) {
	var pData, err = p.MarshalPacket()
	if err != nil {
		return nil, err
	}
	var data = make([]byte, 4+len(pData))
	binary.BigEndian.PutUint32(data[0:4], uint32(len(pData)))
	copy(data[4:], pData)
	return data, nil
}

func (this *TCPProtocol) Unmarshal(r io.Reader) (net4go.Packet, error) {
	var lengthBytes = make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBytes); err != nil {
		return nil, err
	}
	var length = binary.BigEndian.Uint32(lengthBytes)

	var buff = make([]byte, length)
	if _, err := io.ReadFull(r, buff); err != nil {
		return nil, err
	}

	var p = &Packet{}
	if err := p.UnmarshalPacket(buff); err != nil {
		return nil, err
	}
	return p, nil
}

type WSProtocol struct {
}

func (this *WSProtocol) Marshal(p net4go.Packet) ([]byte, error) {
	return json.Marshal(p)
}

func (this *WSProtocol) Unmarshal(r io.Reader) (net4go.Packet, error) {
	var p *Packet
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	return p, nil
}
