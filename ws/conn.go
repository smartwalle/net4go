package ws

import (
	"bytes"
	"github.com/gorilla/websocket"
	"github.com/smartwalle/net4go"
	"net"
	"sync"
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

	closeChan chan struct{}
	closeOnce sync.Once

	writeBuffer chan []byte

	pongWait   time.Duration
	pingPeriod time.Duration
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

	nc.closeChan = make(chan struct{})
	nc.writeBuffer = make(chan []byte, nc.WriteChanSize)

	nc.pongWait = nc.ReadTimeout
	nc.pingPeriod = (nc.pongWait * 9) / 10

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
	select {
	case <-this.closeChan:
		return true
	default:
		return false
	}
}

func (this *wsConn) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.write(w)
	go this.read(w)

	w.Wait()
}

func (this *wsConn) read(w *sync.WaitGroup) {
	this.conn.SetReadLimit(int64(this.ReadBufferSize))

	this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
	this.conn.SetPongHandler(func(string) error {
		this.conn.SetReadDeadline(time.Now().Add(this.pongWait))
		return nil
	})

	w.Done()

	var err error
	var p net4go.Packet
	var msg []byte

ReadLoop:
	for {
		select {
		case <-this.closeChan:
			break ReadLoop
		default:
			var h = this.handler
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
			if p != nil && h != nil {
				if h.OnMessage(this, p) == false {
					break ReadLoop
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

WriteLoop:
	for {
		select {
		case <-this.closeChan:
			break WriteLoop
		case <-ticker.C:
			if err = this.writeMessage(websocket.PingMessage, nil); err != nil {
				break WriteLoop
			}
		default:
			select {
			case p, ok := <-this.writeBuffer:
				if ok == false {
					break WriteLoop
				}

				if _, err = this.Write(p); err != nil {
					break WriteLoop
				}
			case <-time.After(time.Second * 5):
			}
		}
	}

	ticker.Stop()
	this.close(err)
}

func (this *wsConn) AsyncWritePacket(p net4go.Packet, timeout time.Duration) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	return this.AsyncWrite(pData, timeout)
}

func (this *wsConn) WritePacket(p net4go.Packet) (err error) {
	pData, err := this.protocol.Marshal(p)
	if err != nil {
		return err
	}
	_, err = this.Write(pData)
	return err
}

func (this *wsConn) AsyncWrite(b []byte, timeout time.Duration) (err error) {
	select {
	case <-this.closeChan:
		return net4go.ErrConnClosed
	default:
		if timeout == 0 {
			select {
			case this.writeBuffer <- b:
				return nil
			default:
				return net4go.ErrWriteFailed
			}
		}

		select {
		case this.writeBuffer <- b:
			return nil
		case <-time.After(timeout):
			return net4go.ErrWriteFailed
		}
	}
}

func (this *wsConn) Write(b []byte) (n int, err error) {
	if err = this.writeMessage(int(this.messageType), b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (this *wsConn) writeMessage(messageType int, data []byte) (err error) {
	if this.conn == nil {
		return net4go.ErrConnClosed
	}

	if this.WriteTimeout > 0 {
		this.conn.SetWriteDeadline(time.Now().Add(this.WriteTimeout))
	}

	select {
	case <-this.closeChan:
		return net4go.ErrConnClosed
	default:
		this.mu.Lock()
		defer this.mu.Unlock()
		return this.conn.WriteMessage(messageType, data)
	}
}

func (this *wsConn) close(err error) {
	this.closeOnce.Do(func() {
		close(this.closeChan)
		close(this.writeBuffer)

		this.writeBuffer = nil

		this.conn.Close()
		if this.handler != nil {
			this.handler.OnClose(this, err)
		}

		this.data = nil
		this.handler = nil
	})
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
