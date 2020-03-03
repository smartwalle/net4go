package net4go

import (
	"bytes"
	"github.com/gorilla/websocket"
	"net"
	"sync"
	"time"
)

type wsConn struct {
	*connOption

	conn *websocket.Conn

	mu   sync.Mutex
	data map[string]interface{}

	protocol Protocol
	handler  Handler

	isClosed  bool
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
	nc.isClosed = false

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

func (this *wsConn) IsClosed() bool {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.isClosed
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
			break ReadFor
		default:
			//if this.readTimeout > 0 {
			//	this.conn.SetReadDeadline(time.Now().Add(this.readTimeout))
			//}

			_, msg, err = this.conn.ReadMessage()
			if err != nil {
				break ReadFor
			}

			this.mu.Lock()
			if this.isClosed == true {
				this.mu.Unlock()
				break ReadFor
			}

			p, err = this.protocol.Unmarshal(bytes.NewReader(msg))
			if err != nil {
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
				break WriteFor
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
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.writeTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.writeTimeout))
	}
	return this.conn.WriteMessage(messageType, data)
}

func (this *wsConn) close(err error) {
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
	this.protocol = nil
	this.handler = nil
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
