//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

// Windows may allow a dual-stack IPv6 probe alongside an existing IPv4 Vite
// listener. Probe each address family explicitly, keeping both sockets open.
func checkPortAvailable(port int) error {
	address := strconv.Itoa(port)
	// A dual-stack listener can also coexist with both bind probes on Windows.
	// Successful loopback connection is sufficient evidence of an active owner.
	for _, host := range []string{"127.0.0.1", "::1"} {
		endpoint := net.JoinHostPort(host, address)
		conn, err := net.DialTimeout("tcp", endpoint, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return fmt.Errorf("已有服务监听 %s", endpoint)
		}
	}
	v4, err := net.Listen("tcp4", "0.0.0.0:"+address)
	if err != nil {
		return err
	}
	defer v4.Close()
	v6, err := net.Listen("tcp6", "[::]:"+address)
	if err != nil {
		if errors.Is(err, windows.WSAEAFNOSUPPORT) || errors.Is(err, windows.WSAEPROTONOSUPPORT) {
			return nil
		}
		return fmt.Errorf("IPv6: %w", err)
	}
	return v6.Close()
}
