package net4go

import (
	"io"
	"net"
	"sync"
	"time"
)

type Forward struct {
	pool         *sync.Pool
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func NewForward(buffSize int, readTimeout, writeTimeout time.Duration) *Forward {
	if buffSize <= 0 {
		buffSize = 32 * 1024
	}
	var f = &Forward{}
	f.pool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, buffSize)
		},
	}
	f.readTimeout = readTimeout
	f.writeTimeout = writeTimeout
	return f
}

func (this *Forward) Bind(c1, c2 net.Conn) (err error) {
	var errorChan = make(chan error, 1)

	go this.forward(errorChan, c1, c2)
	go this.forward(errorChan, c2, c1)

	for i := 0; i < 2; i++ {
		if err = <-errorChan; err != nil {
			return err
		}
	}
	return nil
}

func (this *Forward) forward(errorChan chan error, src, dst net.Conn) {
	var buf = this.pool.Get().([]byte)
	errorChan <- forward(src, dst, buf, this.readTimeout, this.writeTimeout)
	this.pool.Put(buf)
}

func forward(src, dst net.Conn, buf []byte, readTimeout, writeTimeout time.Duration) (err error) {
	var nr int
	var nw int

	for {
		if readTimeout > 0 {
			if err = src.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
				return err
			}
		}

		nr, err = src.Read(buf)
		if err != nil {
			return err
		}

		if nr > 0 {
			if writeTimeout > 0 {
				if err = dst.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
					return err
				}
			}

			nw, err = dst.Write(buf[0:nr])
			if err != nil {
				return err
			}

			if nr != nw {
				err = io.ErrShortWrite
				return err
			}
		}
	}
	return err
}
