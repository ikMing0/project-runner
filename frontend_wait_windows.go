//go:build windows

package main

import (
	"strings"
	"time"
)

type frontendWait struct {
	parent  string
	backend *run
	cancel  chan struct{}
	done    chan struct{}
}

// Caller holds groupMu. Waiting must not block Stop/Restart/Quit.
func (a *App) scheduleFrontendWait(id string) bool {
	parent, child := strings.CutSuffix(id, ":frontend")
	if !child {
		return false
	}
	a.mu.Lock()
	p, ok := a.projects[parent]
	backend := a.runs[parent]
	if !ok || !p.WaitForBackend || a.statuses[parent].State == "running" {
		a.mu.Unlock()
		return false
	}
	if backend == nil {
		status := Status{ID: id, State: "failed", Error: "前端未启动：后端未运行，请先处理后端问题", StartedAt: time.Now().UnixMilli(), Attempt: 1}
		a.statuses[id], a.history[id] = status, nil
		a.emitStatus(status)
		a.mu.Unlock()
		a.appendLog(id, "system", status.Error)
		return true
	}
	if a.frontendWaits == nil {
		a.frontendWaits = map[string]*frontendWait{}
	}
	w := &frontendWait{parent: parent, backend: backend, cancel: make(chan struct{}), done: make(chan struct{})}
	a.frontendWaits[id] = w
	a.history[id] = nil
	status := Status{ID: id, State: "waiting", StartedAt: time.Now().UnixMilli(), Attempt: 1}
	a.statuses[id] = status
	a.emitStatus(status)
	a.mu.Unlock()
	a.appendLog(id, "system", "等待后端就绪后启动前端；可以点击停止取消等待")
	go a.waitForBackend(id, w)
	return true
}

func (a *App) waitForBackend(id string, w *frontendWait) {
	defer close(w.done)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-w.cancel:
			return
		case <-w.backend.done:
		case <-ticker.C:
		}
		a.groupMu.Lock()
		a.mu.Lock()
		current := a.frontendWaits[id] == w
		backend, state := a.runs[w.parent], a.statuses[w.parent].State
		a.mu.Unlock()
		if !current || a.closing {
			a.groupMu.Unlock()
			return
		}
		if backend != w.backend || state == "failed" || state == "stopped" || state == "unready" {
			a.finishFrontendWait(id, w, "failed", "前端未启动：后端已退出或未就绪。处理后端问题后，可点击补齐启动。")
			a.groupMu.Unlock()
			return
		}
		if state == "running" {
			a.mu.Lock()
			delete(a.frontendWaits, id)
			a.mu.Unlock()
			err := a.startProject(id, false)
			if err != nil {
				a.finishFrontendWait(id, w, "failed", "后端已就绪，但前端启动失败："+err.Error())
			} else {
				a.appendLog(id, "system", "后端已就绪，开始启动前端")
			}
			a.groupMu.Unlock()
			return
		}
		a.groupMu.Unlock()
	}
}

func (a *App) finishFrontendWait(id string, w *frontendWait, state, message string) {
	a.mu.Lock()
	if current, found := a.frontendWaits[id]; found && current != w {
		a.mu.Unlock()
		return
	}
	delete(a.frontendWaits, id)
	status := a.statuses[id]
	status.State, status.PID, status.Error = state, 0, ""
	if state == "failed" {
		status.Error = message
	}
	a.statuses[id] = status
	a.emitStatus(status)
	a.mu.Unlock()
	a.appendLog(id, "system", message)
}

// Caller holds groupMu. nil cancels all; a backend ID cancels its paired waiter.
func (a *App) cancelFrontendWaits(ids []string, message string) bool {
	a.mu.Lock()
	var cancelled []string
	for id, w := range a.frontendWaits {
		matches := ids == nil
		for _, serviceID := range ids {
			if serviceID == id || serviceID == w.parent {
				matches = true
			}
		}
		if !matches {
			continue
		}
		delete(a.frontendWaits, id)
		close(w.cancel)
		status := a.statuses[id]
		status.State, status.Error = "stopped", ""
		a.statuses[id] = status
		a.emitStatus(status)
		cancelled = append(cancelled, id)
	}
	a.mu.Unlock()
	for _, id := range cancelled {
		a.appendLog(id, "system", message)
	}
	return len(cancelled) > 0
}
