package net4go

import (
	"encoding/binary"
	"errors"
	"github.com/smartwalle/queue/block"
	"io"
	"net"
	"sync"
	"time"
)

var (
	ErrSessionClosed = errors.New("session closed")
)

type Packet interface {
	MarshalPacket() ([]byte, error)

	UnmarshalPacket([]byte) error
}

type DefaultPacket struct {
	data  []byte
	pType uint16
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
	OnMessage(Session, Packet)

	OnClose(Session, error)
}

type SessionOptions struct {
	Limiter         Limiter
	WriteTimeout    time.Duration
	ReadTimeout     time.Duration
	ReadBufferSize  int
	WriteBufferSize int
	NoDelay         bool
}

func NewSessionOptions() *SessionOptions {
	var opt = &SessionOptions{}
	opt.WriteTimeout = -1
	opt.ReadTimeout = -1

	opt.ReadBufferSize = -1
	opt.WriteBufferSize = -1

	opt.NoDelay = true
	return opt
}

type SessionOption func(*SessionOptions)

func WithWriteTimeout(timeout time.Duration) SessionOption {
	return func(opt *SessionOptions) {
		if timeout < 0 {
			timeout = 0
		}
		opt.WriteTimeout = timeout
	}
}

func WithReadTimeout(timeout time.Duration) SessionOption {
	return func(opt *SessionOptions) {
		if timeout < 0 {
			timeout = 0
		}
		opt.ReadTimeout = timeout
	}
}

func WithReadBufferSize(size int) SessionOption {
	return func(opt *SessionOptions) {
		//if size < 0 {
		//	size = ConnReadBufferSize
		//}
		opt.ReadBufferSize = size
	}
}

func WithWriteBufferSize(size int) SessionOption {
	return func(opt *SessionOptions) {
		//if size < 0 {
		//	size = ConnWriteBufferSize
		//}
		opt.WriteBufferSize = size
	}
}

func WithNoDelay(noDelay bool) SessionOption {
	return func(opt *SessionOptions) {
		opt.NoDelay = noDelay
	}
}

func WithLimiter(limiter Limiter) SessionOption {
	return func(opt *SessionOptions) {
		opt.Limiter = limiter
	}
}

type Session interface {
	Conn() interface{}

	SetId(int64)

	GetId() int64

	UpdateHandler(handler Handler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	Closed() bool

	AsyncWritePacket(p Packet) (err error)

	WritePacket(p Packet) (err error)

	Close() error
}

type rawSession struct {
	handler  Handler
	conn     net.Conn
	wQueue   block.Queue[[]byte]
	rErr     error
	protocol Protocol
	options  *SessionOptions
	hCond    *sync.Cond
	mu       *sync.Mutex
	data     map[string]interface{}
	id       int64
	closed   bool
}

func NewSession(conn net.Conn, protocol Protocol, handler Handler, opts ...SessionOption) Session {
	var ns = &rawSession{}
	ns.options = NewSessionOptions()
	ns.conn = conn
	ns.protocol = protocol
	ns.handler = handler
	ns.mu = &sync.Mutex{}
	ns.hCond = sync.NewCond(ns.mu)

	for _, opt := range opts {
		if opt != nil {
			opt(ns.options)
		}
	}

	ns.closed = false
	ns.wQueue = block.New[[]byte]()

	if tcpConn, ok := ns.conn.(*net.TCPConn); ok {
		if ns.options.ReadBufferSize > 0 {
			tcpConn.SetReadBuffer(ns.options.ReadBufferSize)
		}

		if ns.options.WriteBufferSize > 0 {
			tcpConn.SetWriteBuffer(ns.options.WriteBufferSize)
		}

		tcpConn.SetNoDelay(ns.options.NoDelay)
	}

	ns.run()

	return ns
}

func (this *rawSession) Conn() interface{} {
	return this.conn
}

func (this *rawSession) SetId(id int64) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.id = id
}

func (this *rawSession) GetId() int64 {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.id
}

func (this *rawSession) UpdateHandler(handler Handler) {
	this.hCond.L.Lock()
	this.handler = handler
	this.hCond.L.Unlock()

	this.hCond.Signal()
}

func (this *rawSession) Set(key string, value interface{}) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
}

func (this *rawSession) Get(key string) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *rawSession) Del(key string) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *rawSession) Closed() bool {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.closed
}

func (this *rawSession) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.writeLoop(w)
	go this.readLoop(w)

	w.Wait()
}

func (this *rawSession) readLoop(w *sync.WaitGroup) {
	w.Done()

	var nPacket Packet
	var nHandler Handler
	var nLimiter = this.options.Limiter

ReadLoop:
	for {
		if this.options.ReadTimeout > 0 {
			this.conn.SetReadDeadline(time.Now().Add(this.options.ReadTimeout))
		}
		nPacket, this.rErr = this.protocol.Unmarshal(this.conn)
		if this.rErr != nil {
			break ReadLoop
		}
		this.conn.SetReadDeadline(time.Time{})

		if nPacket != nil {
			this.hCond.L.Lock()
			nHandler = this.handler
			for nHandler == nil {
				if this.closed {
					this.hCond.L.Unlock()
					break ReadLoop
				}
				this.hCond.Wait()
				nHandler = this.handler
			}
			this.hCond.L.Unlock()

			if nLimiter != nil && nLimiter.Allow() == false {
				break ReadLoop
			}
			nHandler.OnMessage(this, nPacket)
		}
	}
	this.wQueue.Close()
}

func (this *rawSession) writeLoop(w *sync.WaitGroup) {
	w.Done()

	var err error

	var writeList [][]byte
WriteLoop:
	for {
		writeList = writeList[0:0]

		var ok = this.wQueue.Dequeue(&writeList)

		for _, item := range writeList {
			if _, err = this.Write(item); err != nil {
				break WriteLoop
			}
		}

		if ok == false {
			err = this.rErr
			break WriteLoop
		}
	}
	this.close(err)
}

func (this *rawSession) AsyncWritePacket(p Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	return this.AsyncWrite(pData)
}

func (this *rawSession) WritePacket(p Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *rawSession) AsyncWrite(b []byte) (err error) {
	if this.Closed() || this.conn == nil {
		return ErrSessionClosed
	}

	if len(b) == 0 {
		return
	}

	this.wQueue.Enqueue(b)
	return nil
}

func (this *rawSession) Write(b []byte) (n int, err error) {
	if this.Closed() || this.conn == nil {
		return 0, ErrSessionClosed
	}

	if len(b) == 0 {
		return
	}

	this.mu.Lock()
	defer this.mu.Unlock()

	var total = len(b)
	for pos := 0; pos < total; {
		if this.options.WriteTimeout > 0 {
			if err = this.conn.SetWriteDeadline(time.Now().Add(this.options.WriteTimeout)); err != nil {
				return 0, err
			}
		}

		n, err = this.conn.Write(b[pos:])
		pos += n

		if err != nil {
			return pos, err
		}
	}
	err = this.conn.SetWriteDeadline(time.Time{})
	return n, err
}

func (this *rawSession) close(err error) {
	this.mu.Lock()
	if this.closed {
		this.mu.Unlock()
		return
	}
	var nHandler = this.handler
	var nLimiter = this.options.Limiter
	this.handler = nil
	this.options.Limiter = nil
	this.closed = true
	this.mu.Unlock()

	this.conn.Close()
	this.hCond.Signal()

	if nHandler != nil {
		if nLimiter != nil {
			nLimiter.Allow()
		}
		nHandler.OnClose(this, err)
	}

	this.data = nil
}

func (this *rawSession) Close() error {
	this.close(nil)
	return nil
}

func (this *rawSession) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

func (this *rawSession) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}
