package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// No credentials are needed: this check only establishes a TCP connection.
type DependencyCheck struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Required bool   `json:"required"`
}

func validateDependencies(checks []DependencyCheck) ([]DependencyCheck, error) {
	if len(checks) > 16 {
		return nil, fmt.Errorf("最多配置 16 个依赖服务")
	}
	out := append([]DependencyCheck(nil), checks...)
	for i := range out {
		d := &out[i]
		d.Name, d.Host = strings.TrimSpace(d.Name), strings.TrimSpace(d.Host)
		if d.Name == "" || len(d.Name) > 100 || strings.ContainsAny(d.Name, "\r\n\t") {
			return nil, fmt.Errorf("依赖服务 %d：请填写名称（最多 100 字节）", i+1)
		}
		// A bare host only; never treat a URL/userinfo as a connection target.
		if strings.HasPrefix(d.Host, "[") && strings.HasSuffix(d.Host, "]") {
			d.Host = strings.TrimSuffix(strings.TrimPrefix(d.Host, "["), "]")
		}
		if d.Host == "" || len(d.Host) > 253 || strings.ContainsAny(d.Host, " /\\@?#\r\n\t") {
			return nil, fmt.Errorf("依赖 %s：请填写主机名或 IP，不含协议、路径和凭据", d.Name)
		}
		if net.ParseIP(d.Host) == nil {
			for _, label := range strings.Split(strings.TrimSuffix(d.Host, "."), ".") {
				if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
					return nil, fmt.Errorf("依赖 %s：主机名无效", d.Name)
				}
				for _, c := range label {
					if c != '-' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
						return nil, fmt.Errorf("依赖 %s：主机名无效", d.Name)
					}
				}
			}
		}
		if d.Port < 1 || d.Port > 65535 {
			return nil, fmt.Errorf("依赖 %s：端口必须在 1–65535 之间", d.Name)
		}
	}
	return out, nil
}

type dependencyResult struct {
	Check DependencyCheck
	Error error
}

func probeDependencies(checks []DependencyCheck) []dependencyResult {
	results := make([]dependencyResult, len(checks))
	var wg sync.WaitGroup
	limit := make(chan struct{}, 4)
	for i, d := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(d.Host, strconv.Itoa(d.Port)))
			if conn != nil {
				conn.Close()
			}
			results[i] = dependencyResult{d, err}
		}()
	}
	wg.Wait()
	return results
}
