package main

import (
	"sync"
	"time"

	"github.com/miekg/dns"
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

// convenient function
func (dc cache) save(req, res *dns.Msg, c int) {
	dc.set(req.Question[0].Name, c, ttl(res.Answer[0].Header().Ttl))
}

func (dc cache) get(d string) int {
	dc.l.RLock()
	defer dc.l.RUnlock()
	return dc.m[d]
}
