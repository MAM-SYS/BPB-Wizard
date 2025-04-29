package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	setDNS()

	isAndroid, err := checkAndroid()
	if err != nil {
		failMessage("Failed to setup Termux environment", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := login(isAndroid); err != nil {
			failMessage("Failed to login", err)
			return
		}
	}()

	go func() {
		defer wg.Done()
		err := configureBPB(isAndroid)
		if err != nil {
			failMessage("Failed to configure BPB Panel", err)
			return
		}
	}()

	server := &http.Server{Addr: ":8976"}
	http.HandleFunc("/oauth/callback", callback)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			failMessage("Error serving localhost.", err)
			return
		}
	}()

	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Application terminated.")
}
