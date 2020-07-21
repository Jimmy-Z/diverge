/* ip4map is a lookup table optimized map for IPv4
	since a plain IP4 lookup table will require 2^32 approx 4G entries
	and CIDR-hash 2d map(as in ipset) is not so ideal performance wise
so the design will be
	stage 1 is a 2^24 lookup table, which is 16M entries
		each entry is 'vBits' wide
			so the size of the map would be 2^24 * vBits
			should be acceptable in most cases
				for vBits = 2, 4M bytes
				for vBits = 4, 8M bytes
		if the entry value is 0 ~ 2^vBits - 2, the entire /24 block is mapped to that value
			so we only have 2^vBits - 1 different values, not 2^vBits
				vBits = 1 is not acceptable, a map can only map to 1 value is meaningless
		if the entry value is 2^n - 1, consult the 2nd stage for finer/smaller blocks
	stage 2 is the good old CIDR-hash map
		since it only covers blocks smaller than /24 the performance penalty should be small
we actually supports arbitrary vBits and stage 1 bits/width
	vBits should be power of 2 and fits in uint64
*/
package ip4map

import (
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

type IP4Map struct {
	vBits        int
	s1Bits       int
	s1           []uint64
	s2           []map[uint32]int
	vMask        uint64
	s1IndexLBits int
	s1IndexLMask uint32
	s2masks      []uint32
}

func New(vBits, s1Bits int) *IP4Map {
	m := IP4Map{vBits: vBits, s1Bits: s1Bits}
	s1Len := (1 << s1Bits) * vBits / 64
	m.s1 = make([]uint64, s1Len)
	m.s2 = make([]map[uint32]int, 32-s1Bits)
	// frequently used 'constants'
	m.vMask = ((uint64(1) << vBits) - 1)
	m.s1IndexLBits = intLog2(64 / vBits)
	m.s1IndexLMask = ((uint32(1) << m.s1IndexLBits) - 1)
	s2Len := 32 - s1Bits
	m.s2masks = make([]uint32, s2Len)
	m.s2masks[s2Len-1] = 0xffffffff
	for i := s2Len - 2; i >= 0; i-- {
		m.s2masks[i] = m.s2masks[i+1] << 1
	}
	return &m
}

// Set the network with starting address n and length l to value v
func (m *IP4Map) Set(n uint32, l int, v int) {
	// assumes net/len is a valid network and value is within limit
	if l <= m.s1Bits {
		blocks := 1 << (m.s1Bits - l)
		for i := 0; i < blocks; i++ {
			m.s1Set(n+(uint32(i)<<(32-m.s1Bits)), uint64(v))
		}
	} else {
		m.s1Set(n, m.vMask)
		index := l - m.s1Bits - 1
		submap := m.s2[index]
		if submap == nil {
			submap = map[uint32]int{}
			m.s2[index] = submap
		}
		submap[n] = v
	}
}

// Get the value of a given IP address
func (m *IP4Map) Get(ip uint32) int {
	s1 := m.s1Get(ip)
	if s1 != m.vMask {
		return int(s1)
	}
	for i, submap := range m.s2 {
		// log.Printf("i=%d, ip=%08x, mask=%08x, net=%s(%08x)", i, ip, m.s2masks[i], Uint32ToIPStr(ip&m.s2masks[i]), ip&m.s2masks[i])
		if s2, hit := submap[ip&m.s2masks[i]]; hit {
			return s2
		}
	}
	return 0
}

func (m *IP4Map) s1CalcIndex(net uint32) (uint32, uint32) {
	index := net >> (32 - m.s1Bits)
	indexH := index >> m.s1IndexLBits
	indexL := index & m.s1IndexLMask
	offset := indexL * uint32(m.vBits)
	return indexH, offset
}

func (m *IP4Map) s1Set(net uint32, value uint64) {
	indexH, offset := m.s1CalcIndex(net)
	p := &m.s1[indexH]
	*p = (*p &^ (m.vMask << offset)) | (value << offset)
}

func (m *IP4Map) s1Get(ip uint32) uint64 {
	indexH, offset := m.s1CalcIndex(ip)
	// log.Printf("IP: %x\nindexH: %x\noffset: %x\n", ip, indexH, offset)
	p := &m.s1[indexH]
	return (*p >> offset) & m.vMask
}

// SetStr is Set with CIDR format string input
func (m *IP4Map) SetStr(s string, v int) {
	invalid := func(reason string) {
		log.Printf("invalid %s: %s\n", reason, s)
	}
	split := strings.SplitN(s, "/", 2)
	n, ok := IPStrToUint32(split[0])
	if !ok {
		invalid("address")
	}
	l := 32
	if len(split) == 2 {
		var err error
		l, err = strconv.Atoi(split[1])
		if err != nil || l <= 0 || l > 32 {
			invalid("length")
			return
		}
	}
	m.Set(n, l, v)
}

func (m *IP4Map) LoadList(lst io.Reader, v int) error {
	s := bufio.NewScanner(lst)
	for s.Scan() {
		l := s.Text()
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		m.SetStr(l, v)
	}
	return s.Err()
}

func (m *IP4Map) LoadFile(fn string, v int) {
	f, err := os.Open(fn)
	defer f.Close()
	if err != nil {
		log.Println(err)
		return
	}
	err = m.LoadList(f, v)
	if err != nil {
		log.Println("error loading", fn, err)
	}
}

// LoadLists works on a list of filenames
func (m *IP4Map) LoadFiles(files []string) {
	for i, n := range files {
		m.LoadFile(n, i+1)
	}
}

func (m *IP4Map) GetIP(ip net.IP) int {
	u, ok := IPToUint32(ip)
	if !ok {
		return 0
	}
	return m.Get(u)
}

func (m *IP4Map) GetStr(s string) int {
	u, ok := IPStrToUint32(s)
	if !ok {
		return 0
	}
	return m.Get(u)
}

func IPToUint32(ip net.IP) (uint32, bool) {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, false
	}
	return (((((uint32(ip4[0]) << 8) + uint32(ip4[1])) << 8) + uint32(ip4[2])) << 8) + uint32(ip4[3]), true
}

func Uint32ToIP(ip uint32) *net.IP {
	return &net.IP{
		byte(ip >> 24),
		byte(ip >> 16 & 0xff),
		byte(ip >> 8 & 0xff),
		byte(ip & 0xff),
	}
}

func IPStrToUint32(s string) (uint32, bool) {
	ip := net.ParseIP(s)
	if ip == nil {
		return 0, false
	}
	return IPToUint32(ip)
}

func Uint32ToIPStr(ip uint32) string {
	return Uint32ToIP(ip).String()
}

func intLog2(u int) int {
	r := 0
	for u > 1 {
		u >>= 1
		r++
	}
	return r
}
