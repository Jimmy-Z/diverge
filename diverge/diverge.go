package main

import (
	"log"
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

func decisionToStr(dec int) string {
	switch dec {
	case noDecision:
		return "no decision"
	default:
		return names[dec-upstreamX]
	}
}

// TODO: fallback and retry
func exchange(m *dns.Msg, dec int) (r *dns.Msg, rtt time.Duration, err error) {
	return dnsClient.Exchange(m, upstream[dec-upstreamX][0])
}

func handleWith(w dns.ResponseWriter, req *dns.Msg, rcode int) {
	res := new(dns.Msg)
	res.SetRcode(req, rcode)
	w.WriteMsg(res)
}

func handleBy(w dns.ResponseWriter, req *dns.Msg, dec int) {
	res, _, err := exchange(req, dec)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	// log.Printf("Answer %v: %v\n", rtt, res)
	w.WriteMsg(res)
}

func handleDivergeTypeA(w dns.ResponseWriter, req *dns.Msg) int {
	nErr := 0
	for i := 1; i < len(upstream); i++ {
		// 1 -> upstreamA
		decision := i - 1 + upstreamA
		res, _, err := exchange(req, decision)
		if err != nil {
			log.Printf("upstream %s error: %v", decisionToStr(decision), err)
			nErr++
		} else if postChk(res, i-1+ipA) {
			if w != nil {
				w.WriteMsg(res)
			}
			if nErr == 0 {
				cacheSave(decisionCache, req, res, decision)
			}
			return decision
		}
	}
	res, _, err := exchange(req, upstreamX)
	if err != nil {
		log.Printf("upstream %s error: %v", decisionToStr(upstreamX), err)
	} else {
		if w != nil {
			w.WriteMsg(res)
		}
		if nErr == 0 {
			cacheSave(decisionCache, req, res, upstreamX)
		}
		return upstreamX
	}
	return noDecision
}

func handleDivergeTypeOther(w dns.ResponseWriter, req *dns.Msg) {
	qA := new(dns.Msg)
	qA.SetQuestion(req.Question[0].Name, dns.TypeA)
	decision := handleDivergeTypeA(nil, qA)
	if decision == noDecision {
		return
	}
	handleBy(w, req, decision)
}

func handle(w dns.ResponseWriter, req *dns.Msg) {
	// fmt.Printf("req: %v\n", req)
	upstream, rcode := preChk(req)
	log.Printf("\tpreChk: %s, %s\n", decisionToStr(upstream), dns.RcodeToString[rcode])
	if rcode != dns.RcodeSuccess {
		handleWith(w, req, rcode)
		return
	}
	switch upstream {
	case noDecision:
		switch req.Question[0].Qtype {
		case dns.TypeA:
			handleDivergeTypeA(w, req)
		default:
			handleDivergeTypeOther(w, req)
		}
	default:
		handleBy(w, req, upstream)
	}
}

func preChk(req *dns.Msg) (upstream, rcode int) {
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
		// to do: IPv6 PTR is not handled
		ip, ok := ptrName4ToUint32(q.Name)
		if !ok {
			return noDecision, dns.RcodeRefused
		}
		ipV := ipMap.Get(ip)
		switch ipV {
		case ipPrivate:
			return noDecision, dns.RcodeRefused
		case ipUnknown:
			return upstreamX, dns.RcodeSuccess
		default:
			return upstreamA + ipV - ipA, dns.RcodeSuccess
		}
	}
	if block.includes(q.Name) {
		return noDecision, dns.RcodeRefused
	}
	return decisionCache.get(q.Name), dns.RcodeSuccess
}

func filterRR(rrs []dns.RR, v int) (int, []dns.RR) {
	filtered := make([]dns.RR, 0, len(rrs))
	var nA int
	for _, rr := range rrs {
		a, typeA := (rr).(*dns.A)
		if typeA {
			if ipMap.GetIP(a.A) == v {
				nA++
				filtered = append(filtered, rr)
			}
		} else {
			filtered = append(filtered, rr)
		}
	}
	return nA, filtered
}

func postChk(m *dns.Msg, v int) bool {
	var nA int
	nA, m.Answer = filterRR(m.Answer, v)
	_, m.Extra = filterRR(m.Extra, v)
	return nA > 0
}
