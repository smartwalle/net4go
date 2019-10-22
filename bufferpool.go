package net4go

import "sync"

const (
	kDefaultBufferSize = 32 * 1024
)

type BufferPool interface {
	Get() []byte
	Put([]byte)
}

type bufferPool struct {
	*sync.Pool
}

func (this *bufferPool) Get() []byte {
	return this.Pool.Get().([]byte)
}

func (this *bufferPool) Put(v []byte) {
	this.Pool.Put(v)
}

func NewBufferPool(bufferSize int) BufferPool {
	if bufferSize <= 0 {
		bufferSize = kDefaultBufferSize
	}
	var p = &bufferPool{}
	p.Pool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, bufferSize)
		},
	}
	return p
}
