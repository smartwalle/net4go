package ws

import (
	"bytes"
	"github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type wsSession struct {
	*net4go.SessionOption
	messageType MessageType

	conn *websocket.Conn

	id uint64

	data map[string]interface{}

	protocol net4go.Protocol
	handler  net4go.Handler

	closed int32
	mu     *sync.Mutex

	wQueue *net4go.Queue

	//pongWait   time.Duration
	//pingPeriod time.Duration
}

type MessageType int

const (
	Text MessageType = iota + 1
	Binary
)

func NewSession(conn *websocket.Conn, messageType MessageType, protocol net4go.Protocol, handler net4go.Handler, opts ...net4go.Option) net4go.Session {
	var ns = &wsSession{}
	ns.SessionOption = net4go.NewSessionOption()
	ns.messageType = messageType
	ns.conn = conn
	ns.protocol = protocol
	ns.handler = handler

	if messageType != Text && messageType != Binary {
		ns.messageType = Text
	}

	for _, opt := range opts {
		opt.Apply(ns.SessionOption)
	}

	ns.closed = 0
	ns.mu = &sync.Mutex{}
	ns.wQueue = net4go.NewQueue()

	ns.run()

	return ns
}

func (this *wsSession) Conn() interface{} {
	return this.conn
}

func (this *wsSession) SetId(id uint64) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.id = id
}

func (this *wsSession) GetId() uint64 {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.id
}

func (this *wsSession) UpdateHandler(handler net4go.Handler) {
	this.handler = handler
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
	return atomic.LoadInt32(&this.closed) != 0
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
	this.conn.SetReadLimit(int64(this.ReadBufferSize))

	w.Done()

	var err error
	var p net4go.Packet
	var msg []byte

ReadLoop:
	for {
		if this.ReadTimeout > 0 {
			this.conn.SetReadDeadline(time.Now().Add(this.ReadTimeout))
		}
		_, msg, err = this.conn.ReadMessage()
		if err != nil {
			break ReadLoop
		}
		p, err = this.protocol.Unmarshal(bytes.NewReader(msg))
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

func (this *wsSession) writeLoop(w *sync.WaitGroup) {
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

	if this.WriteTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.WriteTimeout))
	}

	this.mu.Lock()
	defer this.mu.Unlock()

	if err = this.conn.WriteMessage(int(this.messageType), b); err != nil {
		return 0, err
	}
	this.conn.SetWriteDeadline(time.Time{})

	return len(b), nil
}

func (this *wsSession) close(err error) {
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
