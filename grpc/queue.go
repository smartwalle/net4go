package grpc

import (
	"github.com/smartwalle/net4go"
	"sync"
)

type Queue struct {
	items []net4go.Packet
	mu    sync.Mutex
	cond  *sync.Cond
}

func (this *Queue) Enqueue(msg net4go.Packet) {
	this.mu.Lock()
	this.items = append(this.items, msg)
	this.mu.Unlock()

	this.cond.Signal()
}

func (this *Queue) Reset() {
	this.items = this.items[0:0]
}

func (this *Queue) Dequeue(items *[]net4go.Packet) {
	this.mu.Lock()
	for len(this.items) == 0 {
		this.cond.Wait()
	}
	this.mu.Unlock()

	this.mu.Lock()
	for _, item := range this.items {
		*items = append(*items, item)
		if item == nil {
			break
		}
	}

	this.Reset()

	this.mu.Unlock()
}

func NewQueue() *Queue {
	var q = &Queue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}
