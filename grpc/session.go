package grpc

import (
	"github.com/smartwalle/net4go"
	"github.com/smartwalle/queue/block"
	"sync"
)

type grpcSession struct {
	stream  Stream
	wQueue  block.Queue[net4go.Packet]
	rErr    error
	handler net4go.Handler
	options *net4go.SessionOption
	data    map[string]interface{}
	hCond   *sync.Cond
	mu      *sync.Mutex
	id      int64
	closed  bool
}

func NewSession(stream Stream, handler net4go.Handler, opts ...net4go.Option) net4go.Session {
	var ns = &grpcSession{}
	ns.options = net4go.NewSessionOption()
	ns.stream = stream
	ns.handler = handler
	ns.mu = &sync.Mutex{}
	ns.hCond = sync.NewCond(ns.mu)

	for _, opt := range opts {
		if opt != nil {
			opt(ns.options)
		}
	}

	ns.closed = false
	ns.wQueue = block.New[net4go.Packet]()

	ns.run()

	return ns
}

func (this *grpcSession) Conn() interface{} {
	return this.stream
}

func (this *grpcSession) SetId(id int64) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.id = id
}

func (this *grpcSession) GetId() int64 {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.id
}

func (this *grpcSession) UpdateHandler(handler net4go.Handler) {
	this.hCond.L.Lock()
	this.handler = handler
	this.hCond.L.Unlock()

	this.hCond.Signal()
}

func (this *grpcSession) Set(key string, value interface{}) {
	this.mu.Lock()
	defer this.mu.Unlock()

	if this.data == nil {
		this.data = make(map[string]interface{})
	}
	this.data[key] = value
}

func (this *grpcSession) Get(key string) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return nil
	}
	return this.data[key]
}

func (this *grpcSession) Del(key string) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.data == nil {
		return
	}
	delete(this.data, key)
}

func (this *grpcSession) Closed() bool {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.closed
}

func (this *grpcSession) run() {
	if this.Closed() {
		return
	}

	var w = &sync.WaitGroup{}
	w.Add(2)

	go this.writeLoop(w)
	go this.readLoop(w)

	w.Wait()
}

func (this *grpcSession) readLoop(w *sync.WaitGroup) {
	w.Done()

	var nPacket net4go.Packet
	var nHandler net4go.Handler
	var nLimiter = this.options.Limiter

ReadLoop:
	for {
		nPacket, this.rErr = this.stream.RecvPacket()
		if this.rErr != nil {
			break ReadLoop
		}

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

func (this *grpcSession) writeLoop(w *sync.WaitGroup) {
	w.Done()

	var err error
	var writeList []net4go.Packet

WriteLoop:
	for {
		writeList = writeList[0:0]

		var ok = this.wQueue.Dequeue(&writeList)

		for _, item := range writeList {
			if err = this.stream.SendPacket(item); err != nil {
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

func (this *grpcSession) AsyncWritePacket(p net4go.Packet) (err error) {
	if this.Closed() || this.stream == nil {
		return net4go.ErrSessionClosed
	}

	if p == nil {
		return
	}

	this.wQueue.Enqueue(p)
	return nil
}

func (this *grpcSession) WritePacket(p net4go.Packet) (err error) {
	if this.Closed() || this.stream == nil {
		return net4go.ErrSessionClosed
	}

	if p == nil {
		return
	}

	this.stream.SendPacket(p)
	return nil
}

func (this *grpcSession) close(err error) {
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

	this.hCond.Signal()
	this.stream.OnClose(err)

	if nHandler != nil {
		if nLimiter != nil {
			nLimiter.Allow()
		}
		nHandler.OnClose(this, err)
	}

	this.data = nil
}

func (this *grpcSession) Close() error {
	this.close(nil)
	return nil
}
