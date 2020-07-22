package main

import "strings"

// since the set is usually small, we should probably use a list and dns.IsSubDomain() instead

type domainSet map[string]struct{}

// domains could comma separated
func newDomainSet(domains ...string) *domainSet {
	s := &domainSet{}
	for _, d := range domains {
		for _, ds := range strings.Split(d, ",") {
			s.append(ds)
		}
	}
	return s
}

func (s domainSet) append(d string) {
	if len(d) == 0 {
		return
	}
	if []byte(d)[len(d)-1] != '.' {
		d = d + "."
	}
	s[d] = struct{}{}
}

func (s domainSet) includes(d string) bool {
	for {
		_, in := s[d]
		if in {
			return true
		}
		dot := strings.IndexByte(d, '.')
		if dot == len(d)-1 || dot == -1 {
			return false
		}
		d = string([]byte(d)[dot+1:])
	}
}
