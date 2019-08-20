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

	kDefaultReadBuffer = 16
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

func WithReadBuffer(size int) Option {
	return OptionFunc(func(c *Conn) {
		if size <= 0 {
			size = kDefaultReadBuffer
		}
		c.readBufferSize = size
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

	writeBuffer chan Packet
	readBuffer  chan Packet

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
	nc.readBufferSize = kDefaultReadBuffer

	for _, opt := range opts {
		opt.Apply(nc)
	}

	nc.closeChan = make(chan struct{})
	nc.writeBuffer = make(chan Packet, nc.writeBufferSize)
	nc.readBuffer = make(chan Packet, nc.readBufferSize)

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
	defer this.mu.Unlock()

	if value != nil {
		if this.data == nil {
			this.data = make(map[string]interface{})
		}
		this.data[key] = value
	}
}

func (this *Conn) Get(key string) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()

	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *Conn) Del(key string) {
	this.mu.Lock()
	defer this.mu.Unlock()

	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *Conn) run() {
	if this.IsClosed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(3)

	go this.write(w)
	go this.read(w)
	go this.handle(w)

	w.Wait()
}

func (this *Conn) read(w *sync.WaitGroup) {
	var err error

	defer func() {
		this.close(err)
	}()

	w.Done()

	var p Packet
	for {
		select {
		case <-this.closeChan:
			return
		default:
			if this.readTimeout > 0 {
				this.conn.SetReadDeadline(time.Now().Add(this.readTimeout))
			}
			p, err = this.protocol.Unmarshal(this.conn)
			if err != nil {
				return
			}
			if p != nil {
				select {
				case this.readBuffer <- p:
				default:
				}
			}
		}
	}
}

func (this *Conn) write(w *sync.WaitGroup) {
	var err error

	defer func() {
		this.close(err)
	}()

	w.Done()

	for {
		select {
		case <-this.closeChan:
			return
		case p, ok := <-this.writeBuffer:
			if ok == false {
				return
			}

			if err = this.WritePacket(p); err != nil {
				return
			}
		}
	}
}

func (this *Conn) handle(w *sync.WaitGroup) {
	defer func() {
		this.close(nil)
	}()

	w.Done()

	for {
		select {
		case <-this.closeChan:
			return
		case p, ok := <-this.readBuffer:
			if ok == false {
				return
			}

			if this.handler != nil {
				if this.handler.OnMessage(this, p) == false {
					return
				}
			}
		}
	}
}

func (this *Conn) AsyncWritePacket(p Packet, timeout time.Duration) (err error) {
	if this.IsClosed() {
		return ErrConnClosed
	}

	if timeout == 0 {
		select {
		case this.writeBuffer <- p:
			return nil
		default:
			return ErrWriteFailed
		}
	}

	select {
	case this.writeBuffer <- p:
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

	if this.writeTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.writeTimeout))
	}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	_, err = this.conn.Write(pData)
	return err
}

func (this *Conn) IsClosed() bool {
	return atomic.LoadInt32(&this.closeFlag) == 1
}

func (this *Conn) close(err error) {
	this.closeOnce.Do(func() {
		atomic.StoreInt32(&this.closeFlag, 1)
		close(this.readBuffer)
		close(this.writeBuffer)
		close(this.closeChan)

		this.readBuffer = nil
		this.writeBuffer = nil

		this.conn.Close()
		if this.handler != nil {
			this.handler.OnClose(this, err)
		}
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
