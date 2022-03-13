package main

import "github.com/miekg/dns"

func handleAsTXT(w dns.ResponseWriter, req *dns.Msg, txt string) {
	rr := dns.TXT{
		Hdr: dns.RR_Header{
			Name:   req.Question[0].Name,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassCHAOS,
			Ttl:    0,
		},
		Txt: []string{txt},
	}
	res := dns.Msg{}
	res.SetRcode(req, dns.RcodeSuccess)
	res.Answer = append(res.Answer, &rr)
	w.WriteMsg(&res)
}

func handleCHAOS(w dns.ResponseWriter, req *dns.Msg) {
	q := &req.Question[0]
	if q.Qtype != dns.TypeTXT {
		handleWith(w, req, dns.RcodeNotImplemented)
	}
	if q.Name == "cache.diverge." {
		handleAsTXT(w, req, decisionCache.info())
		return
	}
	if ip, ok := ptrName4ToUint32(q.Name); ok {
		handleAsTXT(w, req, ip4ToStr(ip))
	} else {
		dec := decisionCache.get(q.Name)
		handleAsTXT(w, req, decisionToStr(dec))
	}
}
