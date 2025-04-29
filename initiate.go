package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func checkAndroid() (isAndroid bool, err error) {
	path := os.Getenv("PATH")
	if runtime.GOOS == "android" || strings.Contains(path, "com.termux") {
		prefix := os.Getenv("PREFIX")
		certPath := filepath.Join(prefix, "etc/tls/cert.pem")
		if err := os.Setenv("SSL_CERT_FILE", certPath); err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
}

func setDNS() {
	http.DefaultTransport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					conn, err := net.Dial("udp", "8.8.8:53")
					if err != nil {
						failMessage("Error dialing DNS (please disconnect your VPN and try again", nil)
						log.Fatal(err)
					}
					return conn, nil
				},
			},
		}
		conn, err := d.DialContext(ctx, network, addr)
		if err != nil {
			failMessage("Error in DNS resolution (please disconnect your VPN and try again)", nil)
			log.Fatal(err)
		}
		return conn, nil
	}

}
