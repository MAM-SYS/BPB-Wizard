package main

import (
	"net/http"
	"sync"
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
		login(isAndroid)
	}()

	go func() {
		defer wg.Done()
		configureBPB(isAndroid)
	}()

	go func() {
		http.HandleFunc("/oauth/callback", callback)
		if err := http.ListenAndServe(":8976", nil); err != nil && err != http.ErrServerClosed {
			failMessage("Error serving localhost.", err)
		}
	}()

	wg.Wait()
}
