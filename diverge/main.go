package main

import (
	"flag"
	"ip4map"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

var (
	listen = flag.String("listen", "127.0.0.1:53",
		"listen on [address]:[port], [address]: can be omitted, which defaults to 127.0.0.1")
	minTTL = flag.Duration("minTTL", 48*time.Hour,
		"minimum TTL for entries in cache")
	redisAddress = flag.String("redis", "",
		"address of redis server, to cache diverge choices, if not specified, a simple in memory cache is used, be aware in this mode TTL is indefinite")
	redisNetwork = flag.String("redis-network", "unix",
		"redis network, for example \"tcp\"")
	redisIndex = flag.Int("redis-index", 0,
		"redis database index")
	flagBlock = flag.String("block", "",
		"comma seperated list of domain names to be blocked")
)

var (
	decisionCache cache
	block         *domainSet
	names         = []string{}
	upstream      = [][]string{}
	ipFiles       = []string{}
	ipMap         *ip4map.IP4Map
	dnsClient     = &dns.Client{}
)

func main() {
	flag.Parse()
	// default to lo
	if !strings.ContainsAny(*listen, ":") {
		*listen = "127.0.0.1:" + *listen
	}
	// nameX uX nameA uA ipA [nameB uB ipB] ...
	if flag.NArg() < 5 || (flag.NArg()-5)%3 != 0 {
		log.Fatalln("invalid parameters")
	}
	// name X, upstream X
	names = append(names, flag.Arg(0))
	upstream = append(upstream, parseUpstream(flag.Arg(1)))
	// name A, upstream A ...
	for i := 2; i+2 < flag.NArg(); i += 3 {
		names = append(names, flag.Arg(i))
		upstream = append(upstream, parseUpstream(flag.Arg(i+1)))
		ipFiles = append(ipFiles, flag.Arg(i+2))
	}

	decisionCache = newCache(*redisNetwork, *redisAddress, *redisIndex)
	block = newDomainSet(*flagBlock)
	ipMap = loadIPMap()

	go func() {
		d := &dns.Server{Addr: *listen, Net: "udp", Handler: dns.HandlerFunc(handle)}
		if err := d.ListenAndServe(); err != nil {
			log.Fatalf("%v\n", err)
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Fatalf("Signal (%v) received, stopping\n", s)
}
