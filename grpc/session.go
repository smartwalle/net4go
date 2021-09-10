package grpc

import (
	"github.com/smartwalle/net4go"
	"sync"
	"sync/atomic"
)

type grpcSession struct {
	*net4go.SessionOption

	stream Stream

	id uint64

	data map[string]interface{}

	handler net4go.Handler
	hCond   *sync.Cond

	closed int32
	mu     *sync.Mutex

	wQueue *Queue
}

func NewSession(stream Stream, handler net4go.Handler) net4go.Session {
	var ns = &grpcSession{}
	ns.SessionOption = net4go.NewSessionOption()
	ns.stream = stream
	ns.handler = handler
	ns.hCond = sync.NewCond(&sync.Mutex{})

	//for _, opt := range opts {
	//	opt.Apply(ns.SessionOption)
	//}

	ns.closed = 0
	ns.mu = &sync.Mutex{}
	ns.wQueue = NewQueue()

	ns.run()

	return ns
}

func (this *grpcSession) Conn() interface{} {
	return this.stream
}

func (this *grpcSession) SetId(id uint64) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.id = id
}

func (this *grpcSession) GetId() uint64 {
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
	return atomic.LoadInt32(&this.closed) != 0
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

	var err error
	var msg interface{}
	var p net4go.Packet

ReadLoop:
	for {
		msg, err = this.stream.RecvPacket()
		if err != nil {
			break ReadLoop
		}

		p, _ = msg.(net4go.Packet)

		if p != nil {
			var h = this.handler
			if h == nil {
				this.hCond.L.Lock()
				for this.handler == nil {
					if this.Closed() {
						this.hCond.L.Unlock()
						break ReadLoop
					}
					this.hCond.Wait()
				}
				h = this.handler
				this.hCond.L.Unlock()
			}

			if h.OnMessage(this, p) == false {
				break ReadLoop
			}
		}
	}
	this.wQueue.Enqueue(nil)
}

func (this *grpcSession) writeLoop(w *sync.WaitGroup) {
	w.Done()

	var err error
	var writeList []net4go.Packet

WriteLoop:
	for {
		writeList = writeList[0:0]

		this.wQueue.Dequeue(&writeList)

		for _, item := range writeList {
			if item == nil {
				break WriteLoop
			}

			if err = this.stream.SendPacket(item); err != nil {
				break WriteLoop
			}
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
	if old := atomic.SwapInt32(&this.closed, 1); old != 0 {
		return
	}

	this.hCond.Signal()

	this.stream.OnClose(err)
	if this.handler != nil {
		this.handler.OnClose(this, err)
	}

	this.data = nil
	this.handler = nil
}

func (this *grpcSession) Close() error {
	this.close(nil)
	return nil
}
