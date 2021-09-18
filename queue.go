package net4go

import (
	"sync"
)

type Queue struct {
	items [][]byte
	cond  *sync.Cond
}

func (this *Queue) Enqueue(msg []byte) {
	this.cond.L.Lock()
	this.items = append(this.items, msg)
	this.cond.L.Unlock()

	this.cond.Signal()
}

func (this *Queue) Dequeue(items *[][]byte) {
	this.cond.L.Lock()
	for len(this.items) == 0 {
		this.cond.Wait()
	}

	for _, item := range this.items {
		*items = append(*items, item)
		if len(item) == 0 {
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
