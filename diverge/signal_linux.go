package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func processSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
loop:
	for {
		s := <-sig
		switch s {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Printf("signal %v, quiting\n", s)
			break loop
		case syscall.SIGUSR1:
			log.Printf("signal %v, reloading IP list files\n", s)
			ipMap = loadIPMap()
		}
	}
}
