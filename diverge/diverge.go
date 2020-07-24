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

func exchange(m *dns.Msg, dec int) (r *dns.Msg, rtt time.Duration, err error) {
	client := &dns.Client{}
	for _, addr := range upstream[dec-upstreamX] {
		r, rtt, err = client.Exchange(m, addr)
		if err != nil {
			continue
		} else {
			break
		}
	}
	return
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

func handleDivergeTypeASeq(w dns.ResponseWriter, req *dns.Msg) int {
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

type response struct {
	msg *dns.Msg
	err error
}

func finalDecision(w dns.ResponseWriter, req, res *dns.Msg, decision, nErr int) {
	if w != nil {
		w.WriteMsg(res)
	}
	if nErr == 0 {
		cacheSave(decisionCache, req, res, decision)
	}
	log.Printf("\tdecision %s: %s", req.Question[0].Name, decisionToStr(decision))
}

func handleDivergeTypeA(w dns.ResponseWriter, req *dns.Msg) int {
	rArray := make([]chan response, len(upstream))
	for i := range upstream {
		decision := i + upstreamX
		rArray[i] = make(chan response, 1)
		go func(req dns.Msg, dec int, r chan<- response) {
			res, _, err := exchange(&req, dec)
			r <- response{res, err}
			close(r)
		}(*req, decision, rArray[i])
	}
	nErr := 0
	for i, r := range rArray[1:] {
		decision := i + upstreamA
		res := <-r
		if res.err != nil {
			log.Printf("\tupstream %s error: %v", decisionToStr(decision), res.err)
			nErr++
		} else if postChk(res.msg, i+ipA) {
			finalDecision(w, req, res.msg, decision, nErr)
			return decision
		}
	}
	res := <-(rArray[0])
	if res.err != nil {
		log.Printf("\tupstream %s error: %v", decisionToStr(upstreamX), res.err)
		nErr++
	} else {
		finalDecision(w, req, res.msg, upstreamX, nErr)
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
	// log.Printf("Query: %v", req)
	if len(req.Question) != 1 {
		log.Printf("unexpected len(req.Question): %d\n", len(req.Question))
		handleWith(w, req, dns.RcodeRefused)
		return
	}
	q := &req.Question[0]
	log.Printf("query: %s %s %s\n", q.Name, dns.ClassToString[q.Qclass], dns.TypeToString[q.Qtype])
	if q.Qclass == dns.ClassCHAOS {
		handleCHAOS(w, req)
		return
	} else if q.Qclass != dns.ClassINET {
		log.Printf("\tquery class not supported: %s\n", dns.ClassToString[q.Qclass])
		handleWith(w, req, dns.RcodeNotImplemented)
		return
	}
	// fmt.Printf("req: %v\n", req)
	upstream, rcode := preChk(q)
	log.Printf("\tpreChk %s: %s, %s\n", q.Name, decisionToStr(upstream), dns.RcodeToString[rcode])
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

func preChk(q *dns.Question) (upstream, rcode int) {
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
		if rr.Header().Rrtype == dns.TypeA {
			if ipMap.GetIP(rr.(*dns.A).A) == v {
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
