package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EziosWJ/weighbridge/server/internal/monitoring"
)

func main() {
	socket := flag.String("socket", "/tmp/weighbridge-host-collector.sock", "local Unix socket path")
	flag.Parse()
	collector, err := monitoring.NewLinuxCollector()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("host collector listening on %s (5s interval; host namespaces required)", *socket)
	if err := monitoring.ServeUnixCollector(ctx, *socket, collector); err != nil {
		log.Fatal(err)
	}
}
