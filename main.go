package main

import (
	_ "embed"
	"net/http"
)

func main() {
	go login()
	go configureBPB()
	http.HandleFunc("/oauth/callback", callback)
	if err := http.ListenAndServe(":8976", nil); err != nil {
		failMessage("Error serving localhost.", err)
	}
}
