package net4go

type Limiter interface {
	Allow() bool
}
