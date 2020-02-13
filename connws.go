package net4go

import (
	"bytes"
	"github.com/gorilla/websocket"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type wsConn struct {
	*connOption

	conn *websocket.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol Protocol
	handler  Handler

	closeFlag uint32
	closeChan chan struct{}

	writeBuffer chan []byte

	pongWait   time.Duration
	pingPeriod time.Duration
}

func NewWsConn(conn *websocket.Conn, protocol Protocol, handler Handler, opts ...Option) Conn {
	var nc = &wsConn{}
	nc.connOption = newConnOption()
	nc.conn = conn
	nc.protocol = protocol
	nc.handler = handler
	nc.closeFlag = 0

	for _, opt := range opts {
		opt.Apply(nc.connOption)
	}

	nc.closeChan = make(chan struct{})
	nc.writeBuffer = make(chan []byte, nc.writeBufferSize)

	nc.pongWait = nc.readTimeout
	nc.pingPeriod = (nc.pongWait * 9) / 10

	nc.run()

	return nc
}

func (this *wsConn) Conn() net.Conn {
	return this.conn.UnderlyingConn()
}

func (this *wsConn) UpdateHandler(handler Handler) {
	this.handler = handler
}

func (this *wsConn) Set(key string, value interface{}) {
	this.mu.Lock()
	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
	this.mu.Unlock()
}

func (this *wsConn) Get(key string) interface{} {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return nil
	}
	var value = this.data[key]
	this.mu.Unlock()
	return value
}

func (this *wsConn) Del(key string) {
	this.mu.Lock()

	if this.data == nil {
		this.mu.Unlock()
		return
	}
	delete(this.data, key)
	this.mu.Unlock()
}

func (this *wsConn) IsClosed() bool {
	return atomic.LoadUint32(&this.closeFlag) == 1
}

func (this *wsConn) run() {
	if this.IsClosed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.write(w)
	go this.read(w)

	w.Wait()
}

func (this *wsConn) read(w *sync.WaitGroup) {
	this.conn.SetReadLimit(this.readLimitSize)
	this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
	this.conn.SetPongHandler(func(string) error {
		this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
		return nil
	})

	w.Done()

	var err error
	var p Packet
	var msg []byte

ReadFor:
	for {
		select {
		case <-this.closeChan:
			break
		default:
			//if this.readTimeout > 0 {
			//	this.conn.SetReadDeadline(time.Now().Add(this.readTimeout))
			//}

			_, msg, err = this.conn.ReadMessage()
			if err != nil {
				break ReadFor
			}

			p, err = this.protocol.Unmarshal(bytes.NewReader(msg))
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

func (this *wsConn) write(w *sync.WaitGroup) {
	var ticker = time.NewTicker(this.pingPeriod)

	w.Done()

	var err error

WriteFor:
	for {
		select {
		case <-this.closeChan:
			break WriteFor
		case <-ticker.C:
			if err = this.writeMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case p, ok := <-this.writeBuffer:
			if ok == false {
				//this.writeMessage(websocket.CloseMessage, []byte{})
				break WriteFor
			}

			if _, err = this.Write(p); err != nil {
				break WriteFor
			}
		}
	}

	ticker.Stop()
	this.close(err)
}

func (this *wsConn) AsyncWritePacket(p Packet, timeout time.Duration) (err error) {
	if this.IsClosed() || this.conn == nil {
		return ErrConnClosed
	}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}

	return this.AsyncWrite(pData, timeout)
}

func (this *wsConn) WritePacket(p Packet) (err error) {
	if this.IsClosed() || this.conn == nil {
		return ErrConnClosed
	}

	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *wsConn) AsyncWrite(b []byte, timeout time.Duration) (err error) {
	if this.IsClosed() || this.conn == nil {
		return ErrConnClosed
	}

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

func (this *wsConn) Write(b []byte) (n int, err error) {
	if this.IsClosed() || this.conn == nil {
		return 0, ErrConnClosed
	}
	if err = this.writeMessage(websocket.TextMessage, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (this *wsConn) writeMessage(messageType int, data []byte) (err error) {
	if this.writeTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.writeTimeout))
	}
	return this.conn.WriteMessage(messageType, data)
}

func (this *wsConn) close(err error) {
	if atomic.CompareAndSwapUint32(&this.closeFlag, 0, 1) {
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
	}
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

func (this *wsConn) SetDeadline(t time.Time) error {
	if err := this.conn.SetReadDeadline(t); err != nil {
		return err
	}
	return this.conn.SetWriteDeadline(t)
}

func (this *wsConn) SetReadDeadline(t time.Time) error {
	return this.conn.SetReadDeadline(t)
}

func (this *wsConn) SetWriteDeadline(t time.Time) error {
	return this.conn.SetWriteDeadline(t)
}
