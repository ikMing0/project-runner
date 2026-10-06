//go:build windows

package main

import (
	"net"
	"strconv"
	"testing"
)

func TestPortCheckDetectsEachAddressFamily(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6", "tcp"} {
		t.Run(network, func(t *testing.T) {
			listener, err := net.Listen(network, ":0")
			if err != nil {
				if network == "tcp6" {
					t.Skip("IPv6 unavailable")
				}
				t.Fatal(err)
			}
			defer listener.Close()
			port := listener.Addr().(*net.TCPAddr).Port
			if err := checkPortAvailable(port); err == nil {
				t.Fatalf("missed %s listener on %d", network, port)
			}
		})
	}
}

func TestPortCheckDoesNotLeaveProbeListening(t *testing.T) {
	port := freePort(t)
	if err := checkPortAvailable(port); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
}
