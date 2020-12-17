package net4go

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrConnClosed  = errors.New("net4go: connection closed")
	ErrWriteFailed = errors.New("net4go: writeLoop failed")
)

type Packet interface {
	MarshalPacket() ([]byte, error)

	UnmarshalPacket([]byte) error
}

type DefaultPacket struct {
	pType uint16
	data  []byte
}

func (this *DefaultPacket) MarshalPacket() ([]byte, error) {
	var data = make([]byte, 2+len(this.data))
	binary.BigEndian.PutUint16(data[0:2], this.pType)
	copy(data[2:], this.data)
	return data, nil
}

func (this *DefaultPacket) UnmarshalPacket(data []byte) error {
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
	var pData, err = p.MarshalPacket()
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
	if err := p.UnmarshalPacket(buff); err != nil {
		return nil, err
	}
	return p, nil
}

type Handler interface {
	OnMessage(Conn, Packet) bool

	OnClose(Conn, error)
}

type ConnOption struct {
	WriteTimeout time.Duration
	ReadTimeout  time.Duration

	ReadBufferSize  int
	WriteBufferSize int

	NoDelay bool
}

func NewConnOption() *ConnOption {
	var opt = &ConnOption{}
	opt.WriteTimeout = -1
	opt.ReadTimeout = -1

	opt.ReadBufferSize = -1
	opt.WriteBufferSize = -1

	opt.NoDelay = true
	return opt
}

type Option interface {
	Apply(conn *ConnOption)
}

type OptionFunc func(*ConnOption)

func (f OptionFunc) Apply(c *ConnOption) {
	f(c)
}

func WithWriteTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *ConnOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.WriteTimeout = timeout
	})
}

func WithReadTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *ConnOption) {
		if timeout < 0 {
			timeout = 0
		}
		c.ReadTimeout = timeout
	})
}

func WithReadBufferSize(size int) Option {
	return OptionFunc(func(c *ConnOption) {
		//if size < 0 {
		//	size = ConnReadBufferSize
		//}
		c.ReadBufferSize = size
	})
}

func WithWriteBufferSize(size int) Option {
	return OptionFunc(func(c *ConnOption) {
		//if size < 0 {
		//	size = ConnWriteBufferSize
		//}
		c.WriteBufferSize = size
	})
}

func WithNoDelay(noDelay bool) Option {
	return OptionFunc(func(c *ConnOption) {
		c.NoDelay = noDelay
	})
}

type Conn interface {
	Conn() net.Conn

	UpdateHandler(handler Handler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	Closed() bool

	AsyncWritePacket(p Packet) (err error)

	WritePacket(p Packet) (err error)

	AsyncWrite(b []byte) (err error)

	Write(b []byte) (n int, err error)

	Close() error

	LocalAddr() net.Addr

	RemoteAddr() net.Addr
}

type rawConn struct {
	*ConnOption

	conn net.Conn

	data map[string]interface{}

	protocol Protocol
	handler  Handler

	closed int32

	wQueue *Queue
}

func NewConn(conn net.Conn, protocol Protocol, handler Handler, opts ...Option) Conn {
	var nc = &rawConn{}
	nc.ConnOption = NewConnOption()
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler

	for _, opt := range opts {
		opt.Apply(nc.ConnOption)
	}

	nc.closed = 0
	nc.wQueue = NewQueue()

	if tcpConn, ok := nc.conn.(*net.TCPConn); ok {
		if nc.ReadBufferSize > 0 {
			tcpConn.SetReadBuffer(nc.ReadBufferSize)
		}

		if nc.WriteBufferSize > 0 {
			tcpConn.SetWriteBuffer(nc.WriteBufferSize)
		}

		tcpConn.SetNoDelay(nc.NoDelay)
	}

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
	return atomic.LoadInt32(&this.closed) != 0
}

func (this *rawConn) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.writeLoop(w)
	go this.readLoop(w)

	w.Wait()
}

func (this *rawConn) readLoop(w *sync.WaitGroup) {
	w.Done()

	var err error
	var p Packet

ReadLoop:
	for {
		if this.ReadTimeout > 0 {
			this.conn.SetReadDeadline(time.Now().Add(this.ReadTimeout))
		}
		p, err = this.protocol.Unmarshal(this.conn)
		if err != nil {
			break ReadLoop
		}
		this.conn.SetReadDeadline(time.Time{})

		var h = this.handler
		if p != nil && h != nil {
			if h.OnMessage(this, p) == false {
				break ReadLoop
			}
		}
	}

	this.wQueue.Enqueue(nil)
}

func (this *rawConn) writeLoop(w *sync.WaitGroup) {
	w.Done()

	var err error

	var writeList [][]byte
WriteLoop:
	for {
		writeList = writeList[0:0]

		this.wQueue.Dequeue(&writeList)

		for _, item := range writeList {
			if len(item) == 0 {
				break WriteLoop
			}

			if _, err = this.Write(item); err != nil {
				break WriteLoop
			}
		}
	}
	this.close(err)
}

func (this *rawConn) AsyncWritePacket(p Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	return this.AsyncWrite(pData)
}

func (this *rawConn) WritePacket(p Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *rawConn) AsyncWrite(b []byte) (err error) {
	if this.Closed() || this.conn == nil {
		return ErrConnClosed
	}

	if len(b) == 0 {
		return
	}

	this.wQueue.Enqueue(b)
	return nil
}

func (this *rawConn) Write(b []byte) (n int, err error) {
	if this.Closed() || this.conn == nil {
		return 0, ErrConnClosed
	}

	if len(b) == 0 {
		return
	}

	if this.WriteTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.WriteTimeout))
	}
	n, err = this.conn.Write(b)
	this.conn.SetWriteDeadline(time.Time{})
	return n, err
}

func (this *rawConn) close(err error) {
	if old := atomic.SwapInt32(&this.closed, 1); old != 0 {
		return
	}

	this.conn.Close()
	if this.handler != nil {
		this.handler.OnClose(this, err)
	}

	this.data = nil
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
