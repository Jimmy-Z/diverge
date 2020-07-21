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
	noDecision = iota
	upstreamX
	upstreamA
)

const (
	ipUnknown = iota
	ipPrivate
	ipA
)

const (
	inAddrArpa = "in-addr.arpa."
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

type diverge struct {
	listen   string
	cache    *cache
	blocked  *domainSet
	ipFiles  []string
	ipMap    *ip4map.IP4Map
	upstream [][]string
	client   *dns.Client
}

func newDiverge(listen string, cachePath string, blocked []string, ipFiles []string, upstream [][]string) *diverge {
	for _, u := range upstream {
		for i, a := range u {
			if !strings.ContainsAny(a, ":") {
				u[i] = a + ":53"
			}
		}
	}
	if !strings.ContainsAny(listen, ":") {
		listen = "127.0.0.1:" + listen
	}
	d := diverge{
		listen,
		newCache(cachePath),
		newDomainSet(blocked),
		ipFiles,
		nil,
		upstream,
		&dns.Client{},
	}
	d.blocked.append("home.arpa.")
	d.reload()
	return &d
}

func (d *diverge) reload() {
	lenSets := len(d.ipFiles)
	var vBits int
	switch {
	case lenSets <= (1<<2)-2:
		vBits = 2
	case lenSets <= (1<<4)-2:
		vBits = 4
	default:
		log.Fatal("too many IP sets:", lenSets)
	}
	ipMap := ip4map.New(vBits, 24)
	for _, s := range specialIPv4 {
		ipMap.SetStr(s, ipPrivate)
	}
	for i, fn := range d.ipFiles {
		ipMap.LoadFile(fn, ipA+i)
	}
	d.ipMap = ipMap
}

func ttl(rrTTL uint32) time.Duration {
	ttl := time.Duration(rrTTL) * rrTTLUnit
	if ttl < minTTL {
		return minTTL
	}
	return ttl
}

// TODO: fallback and retry
func (d *diverge) exchange(m *dns.Msg, dec int) (r *dns.Msg, rtt time.Duration, err error) {
	return d.client.Exchange(m, d.upstream[dec-1][0])
}

func handleWith(w dns.ResponseWriter, req *dns.Msg, rcode int) {
	res := new(dns.Msg)
	res.SetRcode(req, rcode)
	w.WriteMsg(res)
}

func (d *diverge) handleBy(w dns.ResponseWriter, req *dns.Msg, dec int) {
	res, _, err := d.exchange(req, dec)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	// log.Printf("Answer %v: %v\n", rtt, res)
	w.WriteMsg(res)
}

func (d *diverge) handleDivergeTypeA(w dns.ResponseWriter, req *dns.Msg) {
	res, _, err := d.exchange(req, upstreamA)
	if err != nil {
		log.Println(err)
	} else if d.postChk(res, ipA) {
		d.cache.set(req.Question[0].Name, upstreamA, ttl(res.Answer[0].Header().Ttl))
		w.WriteMsg(res)
		return
	}
	res, _, err = d.exchange(req, upstreamX)
	if err != nil {
		log.Println(err)
		return
	}
	d.cache.set(req.Question[0].Name, upstreamX, ttl(res.Answer[0].Header().Ttl))
	w.WriteMsg(res)
}

func (d *diverge) handleDivergeTypeOther(w dns.ResponseWriter, req *dns.Msg) {
	qA := new(dns.Msg)
	qA.SetQuestion(req.Question[0].Name, dns.TypeA)
	res, _, err := d.exchange(qA, upstreamA)
	if err == nil && d.postChk(res, ipA) {
		d.cache.set(req.Question[0].Name, upstreamA, ttl(res.Answer[0].Header().Ttl))
		res, _, err = d.exchange(req, upstreamA)
		if err == nil {
			w.WriteMsg(res)
		}
	} else {
		res, _, err = d.exchange(req, upstreamX)
		if err == nil {
			d.cache.set(req.Question[0].Name, upstreamX, ttl(res.Answer[0].Header().Ttl))
			w.WriteMsg(res)
		}
	}
}

func (d *diverge) handle(w dns.ResponseWriter, req *dns.Msg) {
	// fmt.Printf("req: %v\n", req)
	upstream, rcode := d.preChk(req)
	log.Printf("\tpreChk: %d, %d\n", upstream, rcode)
	if rcode != dns.RcodeSuccess {
		handleWith(w, req, rcode)
		return
	}
	switch upstream {
	case noDecision:
		switch req.Question[0].Qtype {
		case dns.TypeA:
			d.handleDivergeTypeA(w, req)
		default:
			d.handleDivergeTypeOther(w, req)
		}
	default:
		d.handleBy(w, req, upstream)
	}
}

func ptrName4ToUint32(p string) (uint32, bool) {
	if !dns.IsSubDomain(inAddrArpa, p) {
		return 0, false
	}
	s := strings.SplitN(p, ".", 5)
	if len(s) != 5 || s[4] != inAddrArpa {
		return 0, false
	}
	var ip uint32
	for i := 0; i < 4; i++ {
		a, err := strconv.Atoi(s[3-i])
		if err != nil {
			return 0, false
		}
		ip = (ip << 8) + uint32(a)
	}
	return ip, true
}

func (d *diverge) preChk(req *dns.Msg) (upstream, rcode int) {
	// log.Printf("Query: %v", req)
	if len(req.Question) != 1 {
		log.Printf("unexpected len(req.Question): %d\n", len(req.Question))
		return noDecision, dns.RcodeRefused
	}
	q := req.Question[0]
	log.Printf("query: %s %s %s\n", q.Name, dns.ClassToString[q.Qclass], dns.TypeToString[q.Qtype])
	if q.Qclass != dns.ClassINET {
		log.Printf("\tquery class not supported: %s\n", dns.ClassToString[q.Qclass])
		return noDecision, dns.RcodeNotImplemented
	}
	switch q.Qtype {
	case dns.TypeANY:
		log.Print("\tquery type ANY not supported\n")
		return noDecision, dns.RcodeNotImplemented
	case dns.TypePTR:
		ip, ok := ptrName4ToUint32(q.Name)
		if !ok {
			return noDecision, dns.RcodeBadName
		}
		ipV := d.ipMap.Get(ip)
		switch ipV {
		case ipPrivate:
			return noDecision, dns.RcodeRefused
		case ipUnknown:
			return upstreamX, dns.RcodeSuccess
		default:
			return upstreamA + ipV - ipA, dns.RcodeSuccess
		}
	}
	if d.blocked.includes(q.Name) {
		return noDecision, dns.RcodeRefused
	}
	return d.cache.get(q.Name), dns.RcodeSuccess
}

func (d *diverge) postChk(m *dns.Msg, ipV int) bool {

	for _, rr := range m.Answer {
		a, ok := rr.(*dns.A)
		if !ok {
			continue
		}
		if d.ipMap.GetIP(a.A) == ipV {
			return true
		}
	}
	return false
}
