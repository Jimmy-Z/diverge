package main

import (
	"ip4map"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	inAddrARPA = "in-addr.arpa."
	rrTTLUnit  = time.Second
)

// part of IANA IPv4 special-purpose address registry
var specialIPv4 = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"240.0.0.0/4",
	"255.255.255.255/32",
}

func parseUpstream(s string) []string {
	u := strings.Split(s, ",")
	for i, a := range u {
		if !strings.ContainsAny(a, ":") {
			u[i] = a + ":53"
		}
	}
	return u
}

func loadIPMap() *ip4map.IP4Map {
	lenSets := len(ipFiles)
	var vBits int
	switch {
	case lenSets <= (1<<2)-3: // it's -3 instead of -2 since ipPrivate took another spot
		vBits = 2
	case lenSets <= (1<<4)-3:
		vBits = 4
	default:
		log.Fatal("too many IP sets:", lenSets)
	}
	newMap := ip4map.New(vBits, 24)
	for _, s := range specialIPv4 {
		newMap.SetStr(s, ipPrivate)
	}
	for i, fn := range ipFiles {
		newMap.LoadFile(fn, ipA+i)
	}
	return newMap
}

func ptrName4ToUint32(p string) (uint32, bool) {
	if !dns.IsSubDomain(inAddrARPA, p) {
		return 0, false
	}
	s := strings.SplitN(p, ".", 5)
	if len(s) != 5 || len(s[4]) != len(inAddrARPA) {
		return 0, false
	}
	var ip uint32
	for i := 0; i < 4; i++ {
		a, err := strconv.Atoi(s[3-i])
		if err != nil || a < 0 || a > 255 {
			return 0, false
		}
		ip = (ip << 8) + uint32(a)
	}
	return ip, true
}
