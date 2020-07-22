package main

import "github.com/miekg/dns"

func handleAsTXT(w dns.ResponseWriter, req *dns.Msg, name, txt string) {
	rr := dns.TXT{
		Hdr: dns.RR_Header{
			Name:   name,
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
	if q.Name == "cache.diverge." && q.Qtype == dns.TypeTXT {
		handleAsTXT(w, req, q.Name, decisionCache.info())
	} else if q.Qtype == dns.TypeTXT {
		dec := decisionCache.get(q.Name)
		handleAsTXT(w, req, q.Name, decisionToStr(dec))
	} else {
		handleWith(w, req, dns.RcodeNotImplemented)
	}

}
