package main

import (
	"flag"
	"fmt"
	"ip4map"
	"log"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var (
	listen = flag.String("listen", "127.0.0.1:53",
		"[address]:[port] or [port]")
	minTTL = flag.Duration("minTTL", 48*time.Hour,
		"minimum TTL for entries in cache")
	UDPSize = flag.Uint("udp-size",512,
		"maximum UDP size of non-EDNS upstream query")
	redisAddress = flag.String("redis", "",
		"address of redis server, to cache diverge decisions\n"+
			"\ta simple in memory cache is used if omitted\n"+
			"\tbe aware in this mode TTL is indefinite")
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
	// dnsClient     = &dns.Client{}
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
	fmt.Printf("configured with %d upstreams:\n", len(names))
	for i, name := range names {
		fmt.Printf("\t%s: %s\n", name, strings.Join(upstream[i], " "))
	}

	decisionCache = newCache(*redisNetwork, *redisAddress, *redisIndex)
	fmt.Println(decisionCache.info())

	block = newDomainSet(*flagBlock)
	ipMap = loadIPMap()

	fmt.Printf("listen on %s\n", *listen)
	// from the looks of the call stack, no need to wrap handler func in another go routine
	dnsd := &dns.Server{Addr: *listen, Net: "udp", Handler: dns.HandlerFunc(handle)}
	go func() {
		if err := dnsd.ListenAndServe(); err != nil {
			log.Fatalf("%v\n", err)
		}
	}()

	processSignal()

	log.Print("quiting")
	dnsd.Shutdown()
	decisionCache.close()
}
