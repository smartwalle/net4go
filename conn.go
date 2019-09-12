package net4go

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrConnClosed  = errors.New("net4go: connection is closed")
	ErrWriteFailed = errors.New("net4go: write failed")
)

const (
	kDefaultWriteTimeout = 10 * time.Second

	kDefaultReadTimeout = 15 * time.Second

	kDefaultWriteBuffer = 16
)

// --------------------------------------------------------------------------------
type Option interface {
	Apply(conn *rawConn)
}

type OptionFunc func(*rawConn)

func (f OptionFunc) Apply(c *rawConn) {
	f(c)
}

func WithWriteTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *rawConn) {
		if timeout < 0 {
			timeout = 0
		}
		c.writeTimeout = timeout
	})
}

func WithReadTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *rawConn) {
		if timeout < 0 {
			timeout = 0
		}
		c.readTimeout = timeout
	})
}

func WithWriteBuffer(size int) Option {
	return OptionFunc(func(c *rawConn) {
		if size <= 0 {
			size = kDefaultWriteBuffer
		}
		c.writeBufferSize = size
	})
}

// --------------------------------------------------------------------------------
type Conn interface {
	net.Conn

	Conn() net.Conn

	UpdateHandler(handler Handler)

	Set(key string, value interface{})

	Get(key string) interface{}

	Del(key string)

	AsyncWritePacket(p Packet, timeout time.Duration) (err error)

	WritePacket(p Packet) (err error)

	IsClosed() bool
}

// --------------------------------------------------------------------------------
type rawConn struct {
	conn net.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol Protocol
	handler  Handler

	closeFlag int32
	closeOnce sync.Once
	closeChan chan struct{}

	writeBufferSize int

	writeBuffer chan []byte

	writeTimeout time.Duration
	readTimeout  time.Duration
}

func NewConn(conn net.Conn, protocol Protocol, handler Handler, opts ...Option) Conn {
	var nc = &rawConn{}
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler

	nc.writeTimeout = kDefaultWriteTimeout
	nc.readTimeout = kDefaultReadTimeout
	nc.writeBufferSize = kDefaultWriteBuffer

	for _, opt := range opts {
		opt.Apply(nc)
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
	this.mu.Lock()
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
	this.mu.Unlock()
}

func (this *rawConn) Get(key string) interface{} {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return nil
	}
	var value = this.data[key]
	this.mu.Unlock()
	return value
}

func (this *rawConn) Del(key string) {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return
	}
	delete(this.data, key)
	this.mu.Unlock()
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
			break
		default:
			if this.readTimeout > 0 {
				this.conn.SetReadDeadline(time.Now().Add(this.readTimeout))
			}
			p, err = this.protocol.Unmarshal(this.conn)
			if err != nil {
				break ReadFor
			}
			if p != nil && this.handler != nil {
				if this.handler.OnMessage(this, p) == false {
					break ReadFor
				}
			}
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
	if this.IsClosed() {
		return ErrConnClosed
	}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	if timeout == 0 {
		select {
		case this.writeBuffer <- pData:
			return nil
		default:
			return ErrWriteFailed
		}
	}

	select {
	case this.writeBuffer <- pData:
		return nil
	case <-this.closeChan:
		return ErrConnClosed
	case <-time.After(timeout):
		return ErrWriteFailed
	}
}

func (this *rawConn) WritePacket(p Packet) (err error) {
	if this.IsClosed() {
		return ErrConnClosed
	}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *rawConn) IsClosed() bool {
	return atomic.LoadInt32(&this.closeFlag) == 1
}

func (this *rawConn) close(err error) {
	this.closeOnce.Do(func() {
		atomic.StoreInt32(&this.closeFlag, 1)
		close(this.writeBuffer)
		close(this.closeChan)

		this.writeBuffer = nil

		this.conn.Close()
		if this.handler != nil {
			this.handler.OnClose(this, err)
		}

		this.data = nil
		this.protocol = nil
		this.handler = nil
	})
}

// net.Conn interface

func (this *rawConn) Read(p []byte) (n int, err error) {
	if this.IsClosed() {
		return 0, ErrConnClosed
	}

	if this.conn == nil {
		return 0, ErrConnClosed
	}
	return this.conn.Read(p)
}

func (this *rawConn) Write(b []byte) (n int, err error) {
	if this.IsClosed() {
		return 0, ErrConnClosed
	}

	if this.conn == nil {
		return 0, ErrConnClosed
	}

	if this.writeTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.writeTimeout))
	}

	return this.conn.Write(b)
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
