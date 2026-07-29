package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"knsight-go/internal/registry"
)

func main() {
	addr := flag.String("addr", ":8081", "registry listen address")
	flag.Parse()

	store := registry.NewStore()
	server := registry.NewServer(store)
	stop := make(chan struct{})
	server.StartCleanupLoop(5*time.Second, stop)

	log.Printf("registry listening on %s", *addr)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		log.Fatalf("registry server error: %v", err)
	}
}
