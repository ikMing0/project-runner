//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func readyProbe(ctx context.Context, p Project) (bool, string) {
	if _, err := validateHealth(p.Health); err != nil {
		return false, "健康检查配置无效"
	}
	if p.Health == nil || p.Health.URL == "" {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)), 150*time.Millisecond)
		if err != nil {
			return false, "端口尚未监听"
		}
		conn.Close()
		return true, ""
	}
	client := &http.Client{Timeout: 900 * time.Millisecond,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Health.URL, nil)
	if err != nil {
		return false, "健康检查地址无效"
	}
	response, err := client.Do(request)
	if err != nil {
		return false, "健康检查请求未成功"
	}
	response.Body.Close()
	expected := p.Health.ExpectedStatus
	if expected == 0 {
		expected = 200
	}
	return response.StatusCode == expected, fmt.Sprintf("健康检查返回 %d，预期 %d", response.StatusCode, expected)
}

func (a *App) waitForProjectReady(id string, p Project, r *run, stageDone <-chan struct{}, generation uint64) {
	seconds := 90
	if p.Health != nil && p.Health.TimeoutSeconds > 0 {
		seconds = p.Health.TimeoutSeconds
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stageDone:
			cancel()
		case <-ctx.Done():
		}
	}()
	deadline := time.NewTimer(time.Duration(seconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	detail := "服务尚未就绪"
	for {
		select {
		case <-r.done:
			return
		case <-ctx.Done():
			return
		case <-deadline.C:
			a.mu.Lock()
			status := a.statuses[id]
			if a.runs[id] != r || r.stage.Load() != generation || status.State != "starting" {
				a.mu.Unlock()
				return
			}
			status.State = "unready"
			status.Error = fmt.Sprintf("等待就绪超过 %d 秒：%s。进程仍在运行，可查看日志或停止服务。", seconds, detail)
			a.statuses[id] = status
			a.emitStatus(status)
			a.mu.Unlock()
			a.appendLog(id, "system", status.Error)
			// Slow services can still become ready after the warning. Keep
			// observing at a lower frequency until this process stage exits.
			ticker.Reset(2 * time.Second)
		case <-ticker.C:
			if r.stage.Load() != generation {
				return
			}
			ready, message := readyProbe(ctx, p)
			detail = message
			if ready {
				a.markRunningStage(id, r, generation)
				return
			}
		}
	}
}

func (a *App) maybeOpenPage(id string) {
	if a.ctx == nil {
		return
	}
	if url := a.takeReadyPage(id); url != "" {
		runtime.EventsEmit(a.ctx, "project:ready", url)
	}
}

func (a *App) takeReadyPage(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	parentID := id
	if _, ok := a.projects[parentID]; !ok {
		for key := range a.projects {
			if frontendID(key) == id {
				parentID = key
				break
			}
		}
	}
	p, ok := a.projects[parentID]
	if !ok || !p.AutoOpen {
		return ""
	}
	ids := []string{parentID}
	port := p.Port
	if p.Frontend != nil {
		if p.Frontend.PortMode == "none" {
			return ""
		}
		ids = append(ids, frontendID(parentID))
		port = p.Frontend.Port
	} else if p.Kind == "node" && p.PortMode == "none" {
		return ""
	}
	var generation int64
	for _, serviceID := range ids {
		s := a.statuses[serviceID]
		if s.State != "running" {
			return ""
		}
		if s.StartedAt > generation {
			generation = s.StartedAt
		}
	}
	if a.openedRuns == nil {
		a.openedRuns = map[string]int64{}
	}
	if a.openedRuns[parentID] == generation {
		return ""
	}
	a.openedRuns[parentID] = generation
	return fmt.Sprintf("http://127.0.0.1:%d/", port)
}
