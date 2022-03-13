package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func processSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
loop:
	for {
		s := <-sig
		switch s {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Printf("signal %v", s)
			break loop
		}
	}
}
