package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/miekg/dns"
)

func main() {
	// listen cachePath blocked nameX uX nameA uA ipA
	if len(os.Args) != 9 {
		log.Fatalln("invalid parameters")
	}

	listen := os.Args[1]
	cachePath := os.Args[2]
	blocked := strings.Split(os.Args[3], ",")
	nameX := os.Args[4]
	uX := strings.Split(os.Args[5], ",")
	nameA := os.Args[6]
	uA := strings.Split(os.Args[7], ",")
	ipA := os.Args[8]

	div := newDiverge(listen, cachePath, blocked,
		[]string{nameX, nameA}, [][]string{uX, uA}, []string{ipA})

	go func() {
		d := &dns.Server{Addr: div.listen, Net: "udp",
			Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
				div.handle(w, req)
			})}
		if err := d.ListenAndServe(); err != nil {
			log.Fatalf("%v\n", err)
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Fatalf("Signal (%v) received, stopping\n", s)
}
