package net4go

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var (
	ErrConnClosed  = errors.New("net4go: connection closed")
	ErrWriteFailed = errors.New("net4go: write failed")
)

const (
	kDefaultWriteTimeout = 10 * time.Second

	kDefaultReadTimeout = 15 * time.Second

	kDefaultWriteBufferSize = 16

	kDefaultReadLimit = 1024
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
	Apply(conn *connOption)
}

type OptionFunc func(*connOption)

func (f OptionFunc) Apply(c *connOption) {
	f(c)
}

func WithWriteTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *connOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.writeTimeout = timeout
	})
}

func WithReadTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *connOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.readTimeout = timeout
	})
}

func WithWriteBufferSize(size int) Option {
	return OptionFunc(func(c *connOption) {
		if size <= 0 {
			size = kDefaultWriteBufferSize
		}
		c.writeBufferSize = size
	})
}

func WithReadLimitSize(size int64) Option {
	return OptionFunc(func(c *connOption) {
		if size < 0 {
			size = kDefaultReadLimit
		}
		c.readLimitSize = size
	})
}

type connOption struct {
	writeTimeout    time.Duration
	readTimeout     time.Duration
	writeBufferSize int
	readLimitSize   int64
}

func newConnOption() *connOption {
	var opt = &connOption{}
	opt.writeTimeout = kDefaultWriteTimeout
	opt.readTimeout = kDefaultReadTimeout
	opt.writeBufferSize = kDefaultWriteBufferSize
	opt.readLimitSize = kDefaultReadLimit
	return opt
}

type Conn interface {
	Conn() net.Conn

	UpdateHandler(handler Handler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	IsClosed() bool

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
	*connOption

	conn net.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol Protocol
	handler  Handler

	isClosed  bool
	closeChan chan struct{}

	writeBuffer chan []byte
}

func NewConn(conn net.Conn, protocol Protocol, handler Handler, opts ...Option) Conn {
	var nc = &rawConn{}
	nc.connOption = newConnOption()
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler
	nc.isClosed = false

	for _, opt := range opts {
		opt.Apply(nc.connOption)
	}

	nc.closeChan = make(chan struct{})
	nc.writeBuffer = make(chan []byte, nc.writeBufferSize)

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

func (this *rawConn) IsClosed() bool {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.isClosed
}

func (this *rawConn) run() {
	if this.IsClosed() {
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

ReadFor:
	for {
		select {
		case <-this.closeChan:
			break ReadFor
		default:
			if this.readTimeout > 0 {
				this.conn.SetReadDeadline(time.Now().Add(this.readTimeout))
			}
			p, err = this.protocol.Unmarshal(this.conn)
			if err != nil {
				break ReadFor
			}
			this.mu.Lock()
			if this.isClosed == true {
				this.mu.Unlock()
				break ReadFor
			}

			if p != nil && this.handler != nil {
				var h = this.handler
				this.mu.Unlock()
				if h.OnMessage(this, p) == false {
					break ReadFor
				}
				continue ReadFor
			}
			this.mu.Unlock()
		}
	}

	this.close(err)
}

func (this *rawConn) write(w *sync.WaitGroup) {
	w.Done()

	var err error

WriteFor:
	for {
		select {
		case <-this.closeChan:
			break WriteFor
		case p, ok := <-this.writeBuffer:
			if ok == false {
				break WriteFor
			}

			if _, err = this.Write(p); err != nil {
				break WriteFor
			}
		}
	}

	this.close(err)
}

func (this *rawConn) AsyncWritePacket(p Packet, timeout time.Duration) (err error) {
	//if this.IsClosed() || this.conn == nil {
	//	return ErrConnClosed
	//}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	return this.AsyncWrite(pData, timeout)
}

func (this *rawConn) WritePacket(p Packet) (err error) {
	//if this.IsClosed() || this.conn == nil {
	//	return ErrConnClosed
	//}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

// net.Conn interface

//func (this *rawConn) Read(p []byte) (n int, err error) {
//	if this.IsClosed() {
//		return 0, ErrConnClosed
//	}
//
//	if this.conn == nil {
//		return 0, ErrConnClosed
//	}
//	return this.conn.Read(p)
//}

func (this *rawConn) AsyncWrite(b []byte, timeout time.Duration) (err error) {
	//if this.IsClosed() || this.conn == nil {
	//	return ErrConnClosed
	//}

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
	case <-this.closeChan:
		return ErrConnClosed
	case <-time.After(timeout):
		return ErrWriteFailed
	}
}

func (this *rawConn) Write(b []byte) (n int, err error) {
	//if this.IsClosed() || this.conn == nil {
	//	return 0, ErrConnClosed
	//}

	this.mu.Lock()
	defer this.mu.Unlock()

	if this.isClosed || this.conn == nil {
		return 0, ErrConnClosed
	}

	if this.writeTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.writeTimeout))
	}

	return this.conn.Write(b)
}

func (this *rawConn) close(err error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.isClosed == true {
		return
	}

	this.isClosed = true
	close(this.writeBuffer)
	close(this.closeChan)

	this.writeBuffer = nil

	this.conn.Close()
	if this.handler != nil {
		this.handler.OnClose(this, err)
	}

	this.data = nil
	//this.protocol = nil
	this.handler = nil
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
