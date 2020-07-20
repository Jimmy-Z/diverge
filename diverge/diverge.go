package main

import (
	"ip4map"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

const (
	noDecision = iota
	upstreamA
	upstreamB
)

const (
	ipUnknown = iota
	ipPrivate
	ipA
)

const (
	inAddrArpa             = "in-addr.arpa."
	defaultDivergeCacheTTL = 72 * time.Hour
)

type domainSet map[string]struct{}

func newDomainSet(domains []string) *domainSet {
	s := &domainSet{}
	for _, d := range domains {
		s.append(d)
	}
	return s
}

func (s domainSet) append(d string) {
	s[d] = struct{}{}
}

func (s domainSet) include(d string) bool {
	for {
		_, in := s[d]
		if in {
			return true
		}
		dot := strings.IndexByte(d, '.')
		if dot == len(d)-1 {
			return false
		}
		d = string([]byte(d)[dot+1:])
	}
}

// TODO: handle ttl
type divergeCache struct {
	m map[string]int
	l *sync.RWMutex
}

func (dc divergeCache) set(d string, c int, ttl time.Duration) {
	dc.l.Lock()
	defer dc.l.Unlock()
	dc.m[d] = c
}

func (dc divergeCache) get(d string) int {
	dc.l.RLock()
	defer dc.l.RUnlock()
	return dc.m[d]
}

func main() {
	// port blocked domainsA ipA uA uB
	if len(os.Args) != 7 {
		log.Fatalln("invalid parameters")
	}

	port := os.Args[1]

	blocked := &domainSet{}
	blocked.append("home.arpa.")
	for _, d := range strings.Split(os.Args[2], ",") {
		blocked.append(d + ".")
	}

	domainsA := &domainSet{}
	for _, d := range strings.Split(os.Args[3], ",") {
		domainsA.append(d + ".")
	}

	ipMap := ip4map.New(2, 24)
	ipMap.SetStr("10.0.0.0/8", ipPrivate)
	ipMap.SetStr("172.16.0.0/12", ipPrivate)
	ipMap.SetStr("192.168.0.0/16", ipPrivate)
	ipMap.LoadFile(os.Args[4], ipA)

	uA := []string{}
	for _, u := range strings.Split(os.Args[5], ",") {
		uA = append(uA, u+":53")
	}

	uB := []string{}
	for _, u := range strings.Split(os.Args[6], ",") {
		uB = append(uB, u+":53")
	}

	dCache := divergeCache{map[string]int{}, &sync.RWMutex{}}

	client := dns.Client{}

	// TODO: fallback and retry
	exchange := func(m *dns.Msg, addrs []string) (r *dns.Msg, rtt time.Duration, err error) {
		return client.Exchange(m, addrs[0])
	}

	preChk := func(req *dns.Msg) (upstream, rcode int) {
		log.Printf("Query: %v", req)
		if len(req.Question) != 1 {
			log.Printf("unexpected len(req.Question): %d\n", len(req.Question))
			return noDecision, dns.RcodeRefused
		}
		q := req.Question[0]
		switch q.Qtype {
		case dns.TypeANY:
			return noDecision, dns.RcodeNotImplemented
		case dns.TypePTR:
			ip, ok := ptrName4ToUint32(q.Name)
			if !ok {
				return noDecision, dns.RcodeBadName
			}
			switch ipMap.Get(ip) {
			case ipPrivate:
				return noDecision, dns.RcodeRefused
			case ipA:
				return upstreamA, dns.RcodeSuccess
			default:
				return upstreamB, dns.RcodeSuccess
			}
		}
		if blocked.include(q.Name) {
			return noDecision, dns.RcodeRefused
		}
		if domainsA.include(q.Name) {
			return upstreamA, dns.RcodeSuccess
		}
		return dCache.get(q.Name), dns.RcodeSuccess
	}

	handleWith := func(w dns.ResponseWriter, req *dns.Msg, rcode int) {
		res := new(dns.Msg)
		res.SetRcode(req, rcode)
		w.WriteMsg(res)
	}

	handleBy := func(w dns.ResponseWriter, req *dns.Msg, upstream []string) {
		res, rtt, err := exchange(req, upstream)
		if err != nil {
			log.Printf("%v\n", err)
			return
		}
		log.Printf("Answer %v: %v\n", rtt, res)
		w.WriteMsg(res)
	}

	postChk := func(m *dns.Msg) bool {
		for _, rr := range m.Answer {
			a, ok := rr.(*dns.A)
			if !ok {
				continue
			}
			if ipMap.GetIP(a.A) == ipA {
				return true
			}
		}
		return false
	}

	// TODO: async
	handleDivergeA := func(w dns.ResponseWriter, req *dns.Msg) {
		a, _, err := exchange(req, uA)
		if err == nil && postChk(a) {
			dCache.set(req.Question[0].Name, upstreamA, defaultDivergeCacheTTL)
			w.WriteMsg(a)
			return
		}
		dCache.set(req.Question[0].Name, upstreamB, defaultDivergeCacheTTL)
		if err != nil {
			log.Println(err)
		}
		a, _, err = exchange(req, uB)
		if err != nil {
			log.Println(err)
			return
		}
		w.WriteMsg(a)
	}

	// TODO: async
	handleDivergeOther := func(w dns.ResponseWriter, req *dns.Msg) {
		qA := new(dns.Msg)
		qA.SetQuestion(req.Question[0].Name, dns.TypeA)
		a, _, err := exchange(qA, uA)
		if err == nil && postChk(a) {
			dCache.set(req.Question[0].Name, upstreamA, defaultDivergeCacheTTL)
			a, _, err = exchange(req, uA)
			if err == nil {
				w.WriteMsg(a)
			}
		} else {
			dCache.set(req.Question[0].Name, upstreamB, defaultDivergeCacheTTL)
			a, _, err = exchange(req, uB)
			if err == nil {
				w.WriteMsg(a)
			}
		}
	}

	handleDiverge := func(w dns.ResponseWriter, req *dns.Msg) {
		if req.Question[0].Qtype == dns.TypeA {
			handleDivergeA(w, req)
		} else {
			handleDivergeOther(w, req)
		}
	}

	handler := func(w dns.ResponseWriter, req *dns.Msg) {
		// fmt.Printf("req: %v\n", req)
		upstream, rcode := preChk(req)
		log.Printf("\tpreChk: %d, %d\n", upstream, rcode)
		if rcode != dns.RcodeSuccess {
			handleWith(w, req, rcode)
			return
		}
		switch upstream {
		case upstreamA:
			handleBy(w, req, uA)
		case upstreamB:
			handleBy(w, req, uB)
		default:
			handleDiverge(w, req)
		}
	}

	go func() {
		d := &dns.Server{Addr: "127.0.0.1:" + port, Net: "udp", Handler: dns.HandlerFunc(handler)}
		if err := d.ListenAndServe(); err != nil {
			log.Fatalf("%v\n", err)
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Fatalf("Signal (%v) received, stopping\n", s)
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
