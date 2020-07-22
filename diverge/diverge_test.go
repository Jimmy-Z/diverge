package main

import (
	"ip4map"
	"path/filepath"
	"testing"
	"time"
)

func TestDomainSet(t *testing.T) {
	ds := newDomainSet("com.", "net.")
	for _, e := range []struct {
		d string
		r bool
	}{
		{"example.com.", true},
		{"example.net.", true},
		{"example.org.", false},
		{"example.net.uk.", false},
	} {
		r := ds.includes(e.d)
		if r != e.r {
			t.Errorf("ds.include(\"%s\") = %v, expecting %v", e.d, r, e.r)
		}
	}
}

func TestIPConv(t *testing.T) {
	a, _ := ip4map.IPStrToUint32("192.168.1.1")
	b, ok := ptrName4ToUint32("1.1.168.192.in-addr.arpa.")
	if !ok {
		t.Error("unexpected conversion error")
	}
	if a != b {
		t.Errorf("conversion results don't match: %d != %d\n", a, b)
	}
}

func TestIPMap(t *testing.T) {
	ipMap := ip4map.New(2, 24)
	ipMap.SetStr("10.0.0.0/8", ipPrivate)
	ipMap.SetStr("172.16.0.0/12", ipPrivate)
	ipMap.SetStr("192.168.0.0/16", ipPrivate)
	ipMap.LoadFile(filepath.Join("\\", "source", "chnroutes2", "chnroutes.txt"), ipA)

	for _, e := range []struct {
		ip string
		r  int
	}{
		{"1.1.1.1", ipUnknown},
		{"192.168.1.1", ipPrivate},
		{"223.5.5.5", ipA},
	} {
		r := ipMap.GetStr(e.ip)
		if r != e.r {
			t.Errorf("ipMap.Get(\"%s\") = %d, expecting %d", e.ip, r, e.r)
		}
	}
}

func TestRedisCache(t *testing.T) {
	c := newCache("tcp", ":6379", 3)
	c.set("test_a", 1, 1*time.Second)
	t.Log(c.get("test_a"))
	t.Log(c.get("test_b"))
	time.Sleep(2 * time.Second)
	t.Log(c.get("test_a"))
}
