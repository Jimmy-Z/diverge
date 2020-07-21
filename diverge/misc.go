package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	inAddrARPA = "in-addr.arpa."
	homeARPA   = "home.arpa."
	rrTTLUnit  = time.Second
	minTTL     = 24 * time.Hour
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

func ttl(rrTTL uint32) time.Duration {
	ttl := time.Duration(rrTTL) * rrTTLUnit
	if ttl < minTTL {
		return minTTL
	}
	return ttl
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
