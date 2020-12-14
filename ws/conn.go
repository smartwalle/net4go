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

type wsConn struct {
	*net4go.ConnOption
	messageType MessageType

	conn *websocket.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol net4go.Protocol
	handler  net4go.Handler

	closed int32

	wQueue *net4go.Queue

	//pongWait   time.Duration
	//pingPeriod time.Duration
}

type MessageType int

const (
	Text MessageType = iota + 1
	Binary
)

func NewConn(conn *websocket.Conn, messageType MessageType, protocol net4go.Protocol, handler net4go.Handler, opts ...net4go.Option) net4go.Conn {
	var nc = &wsConn{}
	nc.ConnOption = net4go.NewConnOption()
	nc.messageType = messageType
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler

	if messageType != Text && messageType != Binary {
		nc.messageType = Text
	}

	for _, opt := range opts {
		opt.Apply(nc.ConnOption)
	}

	nc.closed = 0
	nc.wQueue = net4go.NewQueue()

	//nc.pongWait = nc.ReadTimeout
	//nc.pingPeriod = (nc.pongWait * 9) / 10

	nc.run()

	return nc
}

func (this *wsConn) Conn() net.Conn {
	return this.conn.UnderlyingConn()
}

func (this *wsConn) UpdateHandler(handler net4go.Handler) {
	this.handler = handler
}

func (this *wsConn) Set(key string, value interface{}) {
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
}

func (this *wsConn) Get(key string) interface{} {
	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *wsConn) Del(key string) {
	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *wsConn) Closed() bool {
	return atomic.LoadInt32(&this.closed) != 0
}

func (this *wsConn) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.writeLoop(w)
	go this.readLoop(w)

	w.Wait()
}

func (this *wsConn) readLoop(w *sync.WaitGroup) {
	this.conn.SetReadLimit(int64(this.ReadBufferSize))

	//this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
	//this.conn.SetPongHandler(func(string) error {
	//	this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
	//	return nil
	//})

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

func (this *wsConn) writeLoop(w *sync.WaitGroup) {
	//var ticker = time.NewTicker(this.pingPeriod)

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

		//select {
		//case <-this.closeChan:
		//	break WriteLoop
		//case <-ticker.C:
		//	if err = this.writeMessage(websocket.PingMessage, nil); err != nil {
		//		break WriteLoop
		//	}
		//default:
		//	select {
		//	case p, ok := <-this.writeBuffer:
		//		if ok == false {
		//			break WriteLoop
		//		}
		//
		//		if _, err = this.Write(p); err != nil {
		//			break WriteLoop
		//		}
		//	case <-time.After(time.Second * 5):
		//	}
		//}
	}

	//ticker.Stop()
	this.close(err)
}

func (this *wsConn) AsyncWritePacket(p net4go.Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	return this.AsyncWrite(pData)
}

func (this *wsConn) WritePacket(p net4go.Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *wsConn) AsyncWrite(b []byte) (err error) {
	if this.Closed() || this.conn == nil {
		return net4go.ErrConnClosed
	}

	if len(b) == 0 {
		return
	}

	this.wQueue.Enqueue(b)
	return nil
}

func (this *wsConn) Write(b []byte) (n int, err error) {
	if this.Closed() || this.conn == nil {
		return 0, net4go.ErrConnClosed
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
	return len(b), nil
}

//func (this *wsConn) writeMessage(messageType int, data []byte) (err error) {
//	if this.conn == nil {
//		return net4go.ErrConnClosed
//	}
//
//	if this.WriteTimeout > 0 {
//		this.conn.SetWriteDeadline(time.Now().Add(this.WriteTimeout))
//	}
//
//	select {
//	case <-this.closeChan:
//		return net4go.ErrConnClosed
//	default:
//		this.mu.Lock()
//		defer this.mu.Unlock()
//		return this.conn.WriteMessage(messageType, data)
//	}
//}

func (this *wsConn) close(err error) {
	if old := atomic.SwapInt32(&this.closed, 1); old != 0 {
		return
	}

	this.conn.Close()
	if this.handler != nil {
		this.handler.OnClose(this, err)
	}

	this.data = nil
	this.handler = nil

	//this.closeOnce.Do(func() {
	//	close(this.closeChan)
	//	close(this.writeBuffer)
	//
	//	this.writeBuffer = nil
	//
	//	this.conn.Close()
	//	if this.handler != nil {
	//		this.handler.OnClose(this, err)
	//	}
	//
	//	this.data = nil
	//	this.handler = nil
	//})
}

func (this *wsConn) Close() error {
	this.close(nil)
	return nil
}

func (this *wsConn) LocalAddr() net.Addr {
	return this.conn.LocalAddr()
}

func (this *wsConn) RemoteAddr() net.Addr {
	return this.conn.RemoteAddr()
}
