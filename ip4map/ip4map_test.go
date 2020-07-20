package ip4map

import (
	"math/rand"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStruct(t *testing.T) {
	test := func(vBits, s1Bits, expectedSize int) {
		m := New(vBits, s1Bits)
		if len(m.s1) != 1<<(m.s1Bits-m.s1IndexLBits) {
			t.Error("len(m.s1) != 1 << (m.s1Bits - m.s1IndexLBits)")
		}
		if len(m.s1)*int(reflect.TypeOf(m.s1[0]).Size()) != expectedSize {
			t.Errorf("expected s1 size for IP4Map(%d,%d) should be %d, got %d\n",
				vBits, s1Bits, expectedSize, len(m.s1)*int(reflect.TypeOf(m.s1[0]).Size()))
		}
		// t.Log(m.s2masks)
	}

	test(2, 24, 4<<20)
	test(4, 24, 8<<20)
}

var (
	PATH = filepath.Join("\\", "source", "chnroutes2", "chnroutes.txt")
)

func test(t *testing.T, vBits, s1Bits int) {
	m := New(vBits, s1Bits)
	m.LoadFile(PATH, 1)
	m.SetStr("10.0.0.0/8", 2)
	m.SetStr("172.16.0.0/12", 2)
	m.SetStr("192.168.0.0/16", 2)
	tests := []struct {
		ip       string
		expected int
	}{
		{"202.38.64.1", 1}, // USTC
		{"218.22.21.1", 1},
		{"218.104.71.1", 1},
		{"1.1.1.1", 0},
		{"8.8.8.8", 0},
		{"9.9.9.9", 0},         // quad9
		{"114.114.114.114", 1}, // 114
		{"223.5.5.5", 1},       // ali
		{"202.101.172.35", 1},  // CT hangzhou
		{"192.168.1.1", 2},
	}
	for _, test := range tests {
		if got := m.GetStr(test.ip); got != test.expected {
			t.Errorf("%s -> %d, expecting %d", test.ip, got, test.expected)
		}
	}
}

func Test24(t *testing.T) {
	test(t, 2, 24)
}

func Test8(t *testing.T) {
	test(t, 2, 8)
}

func Test(t *testing.T) {
	m8 := New(2, 8)
	m24 := New(2, 24)
	m8.LoadFiles([]string{PATH})
	m24.LoadFiles([]string{PATH})
	seed := time.Now().UTC().UnixNano()
	t.Logf("rand seed: %d", seed)
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < 0x100000; i++ {
		ip := r.Uint32()
		if m8.Get(ip) != m24.Get(ip) {
			t.Errorf("m8.Get(%s) = %d but m24.Get() = %d", Uint32ToIPStr(ip), m8.Get(ip), m24.Get(ip))
		}
	}
}

func bench(b *testing.B, vBits, s1Bits int) {
	m := New(vBits, s1Bits)
	m.LoadFiles([]string{PATH})
	for i := 0; i < b.N; i++ {
		m.Get(rand.Uint32())
	}
}

func Benchmark2_8(b *testing.B) {
	bench(b, 2, 8)
}

func Benchmark2_16(b *testing.B) {
	bench(b, 2, 16)
}

func Benchmark2_20(b *testing.B) {
	bench(b, 2, 20)
}

func Benchmark2_24(b *testing.B) {
	bench(b, 2, 24)
}

func Benchmark4_8(b *testing.B) {
	bench(b, 4, 8)
}

func Benchmark4_16(b *testing.B) {
	bench(b, 4, 16)
}

func Benchmark4_20(b *testing.B) {
	bench(b, 4, 20)
}

func Benchmark4_24(b *testing.B) {
	bench(b, 4, 24)
}
