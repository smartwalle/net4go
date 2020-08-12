package internal

import "time"

const (
	ConnWriteTimeout = 10 * time.Second

	ConnReadTimeout = 15 * time.Second

	ConnWriteBufferSize = 16

	ConnReadLimit = 1024
)

type ConnOption struct {
	WriteTimeout    time.Duration
	ReadTimeout     time.Duration
	WriteBufferSize int
	ReadLimitSize   int64
}

func NewConnOption() *ConnOption {
	var opt = &ConnOption{}
	opt.WriteTimeout = ConnWriteTimeout
	opt.ReadTimeout = ConnReadTimeout
	opt.WriteBufferSize = ConnWriteBufferSize
	opt.ReadLimitSize = ConnReadLimit
	return opt
}
