package ws

import (
	"bytes"
	"github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/queue/block"
	"net"
	"sync"
	"time"
)

type wsSession struct {
	protocol    net4go.Protocol
	handler     net4go.Handler
	wQueue      block.Queue[[]byte]
	rErr        error
	conn        *websocket.Conn
	options     *net4go.SessionOption
	hCond       *sync.Cond
	mu          *sync.Mutex
	data        map[string]interface{}
	id          int64
	messageType MessageType
	closed      bool
}

type MessageType int

const (
	Text MessageType = iota + 1
	Binary
)

func NewSession(conn *websocket.Conn, messageType MessageType, protocol net4go.Protocol, handler net4go.Handler, opts ...net4go.Option) net4go.Session {
	var ns = &wsSession{}
	ns.options = net4go.NewSessionOption()
	ns.messageType = messageType
	ns.conn = conn
	ns.protocol = protocol
	ns.handler = handler
	ns.mu = &sync.Mutex{}
	ns.hCond = sync.NewCond(ns.mu)

	if messageType != Text && messageType != Binary {
		ns.messageType = Text
	}

	for _, opt := range opts {
		if opt != nil {
			opt(ns.options)
		}
	}

	ns.closed = false
	ns.wQueue = block.New[[]byte]()

	ns.run()

	return ns
}

func (this *wsSession) Conn() interface{} {
	return this.conn
}

func (this *wsSession) SetId(id int64) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.id = id
}

func (this *wsSession) GetId() int64 {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.id
}

func (this *wsSession) UpdateHandler(handler net4go.Handler) {
	this.hCond.L.Lock()
	this.handler = handler
	this.hCond.L.Unlock()

	this.hCond.Signal()
}

func (this *wsSession) Set(key string, value interface{}) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
}

func (this *wsSession) Get(key string) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *wsSession) Del(key string) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *wsSession) Closed() bool {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.closed
}

func (this *wsSession) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.writeLoop(w)
	go this.readLoop(w)

	w.Wait()
}

func (this *wsSession) readLoop(w *sync.WaitGroup) {
	this.conn.SetReadLimit(int64(this.options.ReadBufferSize))

	w.Done()

	var nPacket net4go.Packet
	var nHandler net4go.Handler
	var nLimiter = this.options.Limiter
	var msg []byte

ReadLoop:
	for {
		if this.options.ReadTimeout > 0 {
			this.conn.SetReadDeadline(time.Now().Add(this.options.ReadTimeout))
		}
		_, msg, this.rErr = this.conn.ReadMessage()
		if this.rErr != nil {
			break ReadLoop
		}
		nPacket, this.rErr = this.protocol.Unmarshal(bytes.NewReader(msg))
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

func (this *wsSession) writeLoop(w *sync.WaitGroup) {
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

func (this *wsSession) AsyncWritePacket(p net4go.Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	return this.AsyncWrite(pData)
}

func (this *wsSession) WritePacket(p net4go.Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *wsSession) AsyncWrite(b []byte) (err error) {
	if this.Closed() || this.conn == nil {
		return net4go.ErrSessionClosed
	}

	if len(b) == 0 {
		return
	}

	this.wQueue.Enqueue(b)
	return nil
}

func (this *wsSession) Write(b []byte) (n int, err error) {
	if this.Closed() || this.conn == nil {
		return 0, net4go.ErrSessionClosed
	}

	if len(b) == 0 {
		return
	}

	if this.options.WriteTimeout > 0 {
		if err = this.conn.SetWriteDeadline(time.Now().Add(this.options.WriteTimeout)); err != nil {
			return 0, err
		}
	}

	this.mu.Lock()
	defer this.mu.Unlock()

	if err = this.conn.WriteMessage(int(this.messageType), b); err != nil {
		return 0, err
	}
	if err = this.conn.SetWriteDeadline(time.Time{}); err != nil {
		return 0, err
	}

	return len(b), nil
}

func (this *wsSession) close(err error) {
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

func (this *wsSession) Close() error {
	this.close(nil)
	return nil
}

func (this *wsSession) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

func (this *wsSession) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}
