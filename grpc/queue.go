package grpc

import (
	"github.com/smartwalle/net4go"
	"sync"
)

type Queue struct {
	items []net4go.Packet
	cond  *sync.Cond
}

func (this *Queue) Enqueue(msg net4go.Packet) {
	this.cond.L.Lock()
	this.items = append(this.items, msg)
	this.cond.L.Unlock()

	this.cond.Signal()
}

func (this *Queue) Dequeue(items *[]net4go.Packet) {
	this.cond.L.Lock()
	for len(this.items) == 0 {
		this.cond.Wait()
	}

	for _, item := range this.items {
		*items = append(*items, item)
		if item == nil {
			break
		}
	}

	this.items = this.items[0:0]
	this.cond.L.Unlock()
}

func NewQueue() *Queue {
	var q = &Queue{}
	q.cond = sync.NewCond(&sync.Mutex{})
	return q
}
