package main

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/miekg/dns"
)

type cache interface {
	set(k string, v int, ttl time.Duration)
	get(k string) int
	info() string
}

func cacheSave(c cache, req, res *dns.Msg, v int) {
	k := req.Question[0].Name
	ex := ttl(res)
	go c.set(k, v, ex)
}

func newCache(network, address string, index int) cache {
	if address == "" {
		return newMapCache()
	}
	return newRedisCache(network, address, index)
}

// a in memory only cache, be aware TTL is ignored
type mapCache struct {
	m map[string]int
	l *sync.RWMutex
}

func newMapCache() *mapCache {
	return &mapCache{map[string]int{}, &sync.RWMutex{}}
}

func (mc *mapCache) set(k string, v int, _ time.Duration) {
	mc.l.Lock()
	defer mc.l.Unlock()
	mc.m[k] = v
}

func (mc *mapCache) get(k string) int {
	mc.l.RLock()
	defer mc.l.RUnlock()
	return mc.m[k]
}

func (mc *mapCache) info() string {
	mc.l.Lock()
	defer mc.l.Unlock()
	return "map: " + strconv.Itoa(len(mc.m)) + " entries"
}

// redis
type redisCache redis.Pool

func newRedisCache(network, address string, index int) *redisCache {
	r := &redis.Pool{
		MaxIdle:     2,
		IdleTimeout: 300 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial(network, address)
			if err != nil {
				return nil, err
			}
			if index != 0 {
				if _, err := conn.Do("SELECT", index); err != nil {
					conn.Close()
					return nil, err
				}
			}
			return conn, nil
		},
	}
	return (*redisCache)(r)
}

func (rc *redisCache) set(k string, v int, ttl time.Duration) {
	conn := (*redis.Pool)(rc).Get()
	defer conn.Close()
	_, err := conn.Do("SETEX", k, int(ttl/time.Second), v)
	if err != nil {
		log.Printf("failed to save %s to cache: %v", k, err)
	}
}

func (rc *redisCache) get(k string) int {
	conn := (*redis.Pool)(rc).Get()
	defer conn.Close()
	r, err := conn.Do("GET", k)
	if err != nil {
		log.Printf("failed to get %s from cache: %v", k, err)
		return noDecision
	}
	if r == nil {
		log.Printf("cache miss: %s", k)
		return noDecision
	}
	i, err := redis.Int(r, nil)
	if err != nil {
		log.Printf("failed to convert result to int: %v", r)
		return noDecision
	}
	return i
}

func (rc *redisCache) info() string {
	conn := (*redis.Pool)(rc).Get()
	defer conn.Close()
	r, err := redis.Int(conn.Do("DBSIZE"))
	if err != nil {
		log.Printf("redis error: %v", err)
		return "redis: error"
	}
	return "redis: " + strconv.Itoa(r) + " entries"
}
