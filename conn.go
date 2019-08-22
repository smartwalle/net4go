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
	Apply(conn *Conn)
}

type OptionFunc func(*Conn)

func (f OptionFunc) Apply(c *Conn) {
	f(c)
}

func WithWriteTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *Conn) {
		if timeout < 0 {
			timeout = 0
		}
		c.writeTimeout = timeout
	})
}

func WithReadTimeout(timeout time.Duration) Option {
	return OptionFunc(func(c *Conn) {
		if timeout < 0 {
			timeout = 0
		}
		c.readTimeout = timeout
	})
}

func WithWriteBuffer(size int) Option {
	return OptionFunc(func(c *Conn) {
		if size <= 0 {
			size = kDefaultWriteBuffer
		}
		c.writeBufferSize = size
	})
}

// --------------------------------------------------------------------------------
type Conn struct {
	conn net.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol Protocol
	handler  Handler

	closeFlag int32
	closeOnce sync.Once
	closeChan chan struct{}

	writeBufferSize int
	readBufferSize  int

	writeBuffer chan []byte

	writeTimeout time.Duration
	readTimeout  time.Duration
}

func NewConn(conn net.Conn, protocol Protocol, handler Handler, opts ...Option) *Conn {
	var nc = &Conn{}
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

func (this *Conn) Conn() net.Conn {
	return this.conn
}

func (this *Conn) SetHandler(handler Handler) {
	this.handler = handler
}

func (this *Conn) Set(key string, value interface{}) {
	this.mu.Lock()
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
	this.mu.Unlock()
}

func (this *Conn) Get(key string) interface{} {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return nil
	}
	var value = this.data[key]
	this.mu.Unlock()
	return value
}

func (this *Conn) Del(key string) {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return
	}
	delete(this.data, key)
	this.mu.Unlock()
}

func (this *Conn) run() {
	if this.IsClosed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.write(w)
	go this.read(w)

	w.Wait()
}

func (this *Conn) read(w *sync.WaitGroup) {
	var err error

	w.Done()

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

func (this *Conn) write(w *sync.WaitGroup) {
	var err error

	w.Done()

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

func (this *Conn) AsyncWritePacket(p Packet, timeout time.Duration) (err error) {
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

func (this *Conn) WritePacket(p Packet) (err error) {
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

func (this *Conn) IsClosed() bool {
	return atomic.LoadInt32(&this.closeFlag) == 1
}

func (this *Conn) close(err error) {
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

func (this *Conn) Read(p []byte) (n int, err error) {
	if this.IsClosed() {
		return 0, ErrConnClosed
	}

	if this.conn == nil {
		return 0, ErrConnClosed
	}
	return this.conn.Read(p)
}

func (this *Conn) Write(b []byte) (n int, err error) {
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

func (this *Conn) Close() error {
	this.close(nil)
	return nil
}

func (this *Conn) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

func (this *Conn) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}

func (this *Conn) SetDeadline(t time.Time) error {
	return this.conn.SetDeadline(t)
}

func (this *Conn) SetReadDeadline(t time.Time) error {
	return this.conn.SetReadDeadline(t)
}

func (this *Conn) SetWriteDeadline(t time.Time) error {
	return this.conn.SetWriteDeadline(t)
}
