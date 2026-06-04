package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
)

var (
	workerPath string
	cachePath  string
	isAndroid  = false
	workerURL  = "https://github.com/bia-pain-bache/BPB-Worker-Panel/releases/latest/download/worker.js"
	VERSION    = "dev"
)

func init() {
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()
	if *showVersion {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	initPaths()
	setDNS()
	checkAndroid()
}

func main() {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		runWizard()
	}()

	wg.Wait()
}
