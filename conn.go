package net4go

import (
	"encoding/binary"
	"errors"
	"github.com/smartwalle/net4go/internal"
	"io"
	"net"
	"sync"
	"time"
)

var (
	ErrConnClosed  = errors.New("net4go: connection closed")
	ErrWriteFailed = errors.New("net4go: write failed")
)

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

type Protocol interface {
	// Marshal 把满足 Packet 接口的对象转换为 []byte
	Marshal(p Packet) ([]byte, error)

	// Unmarshal 从 io.Reader 读取数据，转换为相应的满足 Packet 接口的对象, 具体的转换规则需要由开发者自己实现
	Unmarshal(r io.Reader) (Packet, error)
}

type DefaultProtocol struct {
}

func (this *DefaultProtocol) Marshal(p Packet) ([]byte, error) {
	var pData, err = p.Marshal()
	if err != nil {
		return nil, err
	}
	var data = make([]byte, 4+len(pData))
	binary.BigEndian.PutUint32(data[0:4], uint32(len(pData)))
	copy(data[4:], pData)
	return data, nil
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

type Handler interface {
	OnMessage(Conn, Packet) bool

	OnClose(Conn, error)
}

type Option interface {
	Apply(conn *internal.ConnOption)
}

type OptionFunc func(*internal.ConnOption)

func (f OptionFunc) Apply(c *internal.ConnOption) {
	f(c)
}

func WithWriteTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *internal.ConnOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.WriteTimeout = timeout
	})
}

func WithReadTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *internal.ConnOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.ReadTimeout = timeout
	})
}

func WithWriteBufferSize(size int) Option {
	return OptionFunc(func(c *internal.ConnOption) {
		if size <= 0 {
			size = internal.ConnWriteBufferSize
		}
		c.WriteBufferSize = size
	})
}

func WithReadLimitSize(size int64) Option {
	return OptionFunc(func(c *internal.ConnOption) {
		if size < 0 {
			size = internal.ConnReadLimit
		}
		c.ReadLimitSize = size
	})
}

type Conn interface {
	Conn() net.Conn

	UpdateHandler(handler Handler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	Closed() bool

	AsyncWritePacket(p Packet, timeout time.Duration) (err error)

	WritePacket(p Packet) (err error)

	AsyncWrite(b []byte, timeout time.Duration) (err error)

	// net.Conn 除去 Read()
	Write(b []byte) (n int, err error)

	Close() error

	LocalAddr() net.Addr

	RemoteAddr() net.Addr

	SetDeadline(t time.Time) error

	SetReadDeadline(t time.Time) error

	SetWriteDeadline(t time.Time) error
}

type rawConn struct {
	*internal.ConnOption

	conn net.Conn

	data map[string]interface{}

	protocol Protocol
	handler  Handler

	closeChan chan struct{}
	closeOnce sync.Once

	writeBuffer chan []byte
}

func NewConn(conn net.Conn, protocol Protocol, handler Handler, opts ...Option) Conn {
	var nc = &rawConn{}
	nc.ConnOption = internal.NewConnOption()
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler

	for _, opt := range opts {
		opt.Apply(nc.ConnOption)
	}

	nc.closeChan = make(chan struct{})
	nc.writeBuffer = make(chan []byte, nc.WriteBufferSize)

	nc.run()

	return nc
}

func (this *rawConn) Conn() net.Conn {
	return this.conn
}

func (this *rawConn) UpdateHandler(handler Handler) {
	this.handler = handler
}

func (this *rawConn) Set(key string, value interface{}) {
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
}

func (this *rawConn) Get(key string) interface{} {
	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *rawConn) Del(key string) {
	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *rawConn) Closed() bool {
	select {
	case <-this.closeChan:
		return true
	default:
		return false
	}
}

func (this *rawConn) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.write(w)
	go this.read(w)

	w.Wait()
}

func (this *rawConn) read(w *sync.WaitGroup) {
	w.Done()

	var err error
	var p Packet

ReadLoop:
	for {
		select {
		case <-this.closeChan:
			break ReadLoop
		default:
			var h = this.handler
			if this.ReadTimeout > 0 {
				this.conn.SetReadDeadline(time.Now().Add(this.ReadTimeout))
			}
			p, err = this.protocol.Unmarshal(this.conn)
			if err != nil {
				break ReadLoop
			}

			if p != nil && h != nil {
				if h.OnMessage(this, p) == false {
					break ReadLoop
				}
			}
		}
	}

	this.close(err)
}

func (this *rawConn) write(w *sync.WaitGroup) {
	w.Done()

	var err error

WriteLoop:
	for {
		select {
		case <-this.closeChan:
			break WriteLoop
		case p, ok := <-this.writeBuffer:
			if ok == false {
				break WriteLoop
			}

			if _, err = this.Write(p); err != nil {
				break WriteLoop
			}
		}
	}

	this.close(err)
}

func (this *rawConn) AsyncWritePacket(p Packet, timeout time.Duration) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	return this.AsyncWrite(pData, timeout)
}

func (this *rawConn) WritePacket(p Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

// net.Conn interface

//func (this *rawConn) Read(p []byte) (n int, err error) {
//	if this.Closed() {
//		return 0, ErrConnClosed
//	}
//
//	if this.conn == nil {
//		return 0, ErrConnClosed
//	}
//	return this.conn.Read(p)
//}

func (this *rawConn) AsyncWrite(b []byte, timeout time.Duration) (err error) {
	select {
	case <-this.closeChan:
		return ErrConnClosed
	default:
		if timeout == 0 {
			select {
			case this.writeBuffer <- b:
				return nil
			default:
				return ErrWriteFailed
			}
		}

		select {
		case this.writeBuffer <- b:
			return nil
		case <-time.After(timeout):
			return ErrWriteFailed
		}
	}
}

func (this *rawConn) Write(b []byte) (n int, err error) {
	//if this.Closed() || this.conn == nil {
	//	return 0, ErrConnClosed
	//}

	if this.conn == nil {
		return 0, ErrConnClosed
	}

	if this.WriteTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.WriteTimeout))
	}

	select {
	case <-this.closeChan:
		return 0, ErrConnClosed
	default:
		return this.conn.Write(b)
	}
}

func (this *rawConn) close(err error) {
	this.closeOnce.Do(func() {
		close(this.closeChan)
		close(this.writeBuffer)

		this.conn.Close()
		if this.handler != nil {
			this.handler.OnClose(this, err)
		}

		this.data = nil
		this.handler = nil
	})
}

func (this *rawConn) Close() error {
	this.close(nil)
	return nil
}

func (this *rawConn) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

func (this *rawConn) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}

func (this *rawConn) SetDeadline(t time.Time) error {
	return this.conn.SetDeadline(t)
}

func (this *rawConn) SetReadDeadline(t time.Time) error {
	return this.conn.SetReadDeadline(t)
}

func (this *rawConn) SetWriteDeadline(t time.Time) error {
	return this.conn.SetWriteDeadline(t)
}
