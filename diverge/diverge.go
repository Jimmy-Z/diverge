package main

import (
	"ip4map"
	"log"
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

type diverge struct {
	listen   string
	cache    *cache
	blocked  *domainSet
	names    []string
	upstream [][]string
	ipFiles  []string
	ipMap    *ip4map.IP4Map
	client   *dns.Client
}

func newDiverge(listen, cachePath string, blocked, names []string, upstream [][]string, ipFiles []string) *diverge {
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
		names,
		upstream,
		ipFiles,
		nil,
		&dns.Client{},
	}
	d.blocked.append(homeARPA)
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

func decisionToStr(names []string, dec int) string {
	switch dec {
	case noDecision:
		return "no decision"
	default:
		return names[dec-upstreamX]
	}
}

// TODO: fallback and retry
func (d *diverge) exchange(m *dns.Msg, dec int) (r *dns.Msg, rtt time.Duration, err error) {
	return d.client.Exchange(m, d.upstream[dec-upstreamX][0])
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
	} else if postChk(res, d.ipMap, ipA) {
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
	if err == nil && postChk(res, d.ipMap, ipA) {
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
	upstream, rcode := preChk(req, d.cache, d.blocked, d.ipMap)
	log.Printf("\tpreChk: %s, %s\n", decisionToStr(d.names, upstream), dns.RcodeToString[rcode])
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

func preChk(req *dns.Msg, c *cache, b *domainSet, m *ip4map.IP4Map) (upstream, rcode int) {
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
		ipV := m.Get(ip)
		switch ipV {
		case ipPrivate:
			return noDecision, dns.RcodeRefused
		case ipUnknown:
			return upstreamX, dns.RcodeSuccess
		default:
			return upstreamA + ipV - ipA, dns.RcodeSuccess
		}
	}
	if b.includes(q.Name) {
		return noDecision, dns.RcodeRefused
	}
	return c.get(q.Name), dns.RcodeSuccess
}

func filterRR(rrs []dns.RR, m *ip4map.IP4Map, v int) (int, []dns.RR) {
	filtered := make([]dns.RR, 0, len(rrs))
	var nA int
	for _, rr := range rrs {
		a, typeA := (rr).(*dns.A)
		if typeA {
			if m.GetIP(a.A) == v {
				nA++
				filtered = append(filtered, rr)
			}
		} else {
			filtered = append(filtered, rr)
		}
	}
	return nA, filtered
}

func postChk(m *dns.Msg, im *ip4map.IP4Map, v int) bool {
	var nA int
	nA, m.Answer = filterRR(m.Answer, im, v)
	_, m.Extra = filterRR(m.Extra, im, v)
	return nA > 0
}
