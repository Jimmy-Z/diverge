package main

import (
	"sync"
	"time"
)

// TODO: handle ttl, non-volatile, dump
type cache struct {
	m map[string]int
	l *sync.RWMutex
}

func newCache(_ string) *cache {
	return &cache{map[string]int{}, &sync.RWMutex{}}
}

func (dc cache) set(d string, c int, _ time.Duration) {
	dc.l.Lock()
	defer dc.l.Unlock()
	dc.m[d] = c
}

func (dc cache) get(d string) int {
	dc.l.RLock()
	defer dc.l.RUnlock()
	return dc.m[d]
}
