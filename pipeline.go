package net4go

import (
	"io"
	"net"
	"time"
)

type Pipeline interface {
	Bind(c1, c2 net.Conn) (err error)
}

type pipeline struct {
	pool         BufferPool
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func NewPipeline(bufferSize int, readTimeout, writeTimeout time.Duration) Pipeline {
	var p = &pipeline{}
	p.pool = NewBufferPool(bufferSize)
	p.readTimeout = readTimeout
	p.writeTimeout = writeTimeout
	return p
}

func (this *pipeline) Bind(c1, c2 net.Conn) (err error) {
	var errorChan = make(chan error, 1)

	go this.bind(errorChan, c1, c2)
	go this.bind(errorChan, c2, c1)

	for i := 0; i < 2; i++ {
		if err = <-errorChan; err != nil {
			return err
		}
	}
	return nil
}

func (this *pipeline) bind(errorChan chan error, src, dst net.Conn) {
	var buf = this.pool.Get()
	errorChan <- pipe(src, dst, buf, this.readTimeout, this.writeTimeout)
	this.pool.Put(buf)
}

func pipe(src, dst net.Conn, buf []byte, readTimeout, writeTimeout time.Duration) (err error) {
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

		src.SetReadDeadline(time.Time{})

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

			dst.SetWriteDeadline(time.Time{})

			if nr != nw {
				err = io.ErrShortWrite
				return err
			}
		}
	}
	return err
}
