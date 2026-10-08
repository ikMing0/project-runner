package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

type HealthConfig struct {
	URL            string `json:"url"`
	ExpectedStatus int    `json:"expectedStatus"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func validateHealth(h *HealthConfig) (*HealthConfig, error) {
	if h == nil {
		return nil, nil
	}
	copy := *h
	if copy.TimeoutSeconds == 0 {
		copy.TimeoutSeconds = 90
	}
	if copy.ExpectedStatus == 0 {
		copy.ExpectedStatus = 200
	}
	if copy.TimeoutSeconds < 5 || copy.TimeoutSeconds > 600 {
		return nil, fmt.Errorf("就绪等待时间必须在 5–600 秒之间")
	}
	if copy.ExpectedStatus < 200 || copy.ExpectedStatus > 399 {
		return nil, fmt.Errorf("健康检查的预期状态码必须在 200–399 之间")
	}
	copy.URL = strings.TrimSpace(copy.URL)
	if copy.URL != "" {
		u, err := url.Parse(copy.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
			return nil, fmt.Errorf("健康检查需要有效的 HTTP/HTTPS 地址，不包含用户名、密码或片段")
		}
		ip := net.ParseIP(u.Hostname())
		if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("健康检查地址必须指向本机 localhost 或回环 IP")
		}
	}
	return &copy, nil
}
