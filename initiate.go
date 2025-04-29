package main

import (
	"context"
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
					return net.Dial("udp", "8.8.8.8:53")
				},
			},
		}
		return d.DialContext(ctx, network, addr)
	}
}
