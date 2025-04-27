package main

import (
	_ "embed"
	"fmt"
	"net/http"
)

func main() {
	go login()
	go configureBPB()
	http.HandleFunc("/oauth/callback", callback)
	err := http.ListenAndServe(":8976", nil)
	fmt.Println(err)
}
