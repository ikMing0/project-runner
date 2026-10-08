//go:build windows

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func readyGate(t *testing.T) (*httptest.Server, *atomic.Bool) {
	t.Helper()
	ready := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)
	return server, ready
}
func orderedFixture(t *testing.T) (Project, *App, *atomic.Bool) {
	p, a := pairedFixture(t)
	server, ready := readyGate(t)
	p.WaitForBackend = true
	p.Health = &HealthConfig{URL: server.URL, TimeoutSeconds: 5, ExpectedStatus: 200}
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	return p, a, ready
}
func TestFrontendWaitsForBackendHealthAndCanCancelThenStartIndependently(t *testing.T) {
	p, a, ready := orderedFixture(t)
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "waiting")
	waitStatus(t, a, p.ID, "starting")
	if exists(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("frontend launched before backend readiness")
	}
	a.mu.Lock()
	w := a.frontendWaits[frontendID(p.ID)]
	a.mu.Unlock()
	at := time.Now()
	if err := a.StopService(frontendID(p.ID)); err != nil {
		t.Fatal(err)
	}
	if time.Since(at) > time.Second {
		t.Fatal("stopping frontend wait blocked")
	}
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("waiter leaked after cancellation")
	}
	if workflowStatus(a, p.ID).State != "starting" {
		t.Fatal("canceling wait stopped backend")
	}
	if err := a.StartService(frontendID(p.ID)); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "running")
	ready.Store(true)
	waitStatus(t, a, p.ID, "running")
}
func TestOrderedGroupStartsFrontendAfterReadyAndRestartRemovesOldWaiters(t *testing.T) {
	p, a, ready := orderedFixture(t)
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "waiting")
	a.mu.Lock()
	old := a.frontendWaits[frontendID(p.ID)]
	a.mu.Unlock()
	if err := a.RestartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old waiter survived restart")
	}
	waitStatus(t, a, frontendID(p.ID), "waiting")
	if exists(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("old waiter launched frontend")
	}
	ready.Store(true)
	backend := waitStatus(t, a, p.ID, "running")
	front := waitStatus(t, a, frontendID(p.ID), "running")
	if front.StartedAt <= backend.StartedAt {
		t.Fatal("frontend start preceded backend")
	}
	a.mu.Lock()
	left := len(a.frontendWaits)
	a.mu.Unlock()
	if left != 0 {
		t.Fatal("completed wait retained")
	}
}
func TestBackendFailureAndUnreadyNeverLaunchOrderedFrontend(t *testing.T) {
	t.Run("startup failure", func(t *testing.T) {
		p, a, _ := orderedFixture(t)
		p.Environment["RUNNER_TEST_APP_ERROR"] = "database"
		if _, err := a.SaveProject(p); err != nil {
			t.Fatal(err)
		}
		if err := a.StartProject(p.ID); err != nil {
			t.Fatal(err)
		}
		waitStatus(t, a, p.ID, "failed")
		status := waitStatus(t, a, frontendID(p.ID), "failed")
		if status.PID != 0 || exists(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"]) {
			t.Fatal("frontend launched after backend failure")
		}
	})
	t.Run("readiness timeout", func(t *testing.T) {
		p, a, ready := orderedFixture(t)
		if err := a.StartProject(p.ID); err != nil {
			t.Fatal(err)
		}
		waitStatus(t, a, p.ID, "unready")
		waitStatus(t, a, frontendID(p.ID), "failed")
		if exists(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"]) {
			t.Fatal("frontend launched after health timeout")
		}
		ready.Store(true)
		waitStatus(t, a, p.ID, "running")
		if err := a.StartProject(p.ID); err != nil {
			t.Fatal(err)
		}
		waitStatus(t, a, frontendID(p.ID), "running")
	})
}
func TestStopOrderedGroupDuringBuildCancelsWaitPromptly(t *testing.T) {
	p, a, _ := orderedFixture(t)
	p.Environment["RUNNER_TEST_BUILD"] = "wait"
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "building")
	waitStatus(t, a, frontendID(p.ID), "waiting")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	if workflowStatus(a, frontendID(p.ID)).State != "stopped" {
		t.Fatal("frontend wait not canceled")
	}
}
func TestManagedProcessMetricsIncludeChildrenAndClearAfterStop(t *testing.T) {
	p, a := pairedFixture(t)
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	waitStatus(t, a, frontendID(p.ID), "running")
	first := a.GetProcessMetrics()
	if len(first) != 2 {
		t.Fatalf("metrics: %+v", first)
	}
	for _, value := range first {
		if value.Error != "" || value.ProcessCount < 1 || value.MemoryBytes == 0 || value.Partial {
			t.Fatalf("unreadable metrics: %+v", value)
		}
		if value.ID == frontendID(p.ID) && value.ProcessCount < 2 {
			t.Fatal("npm child omitted")
		}
	}
	time.Sleep(100 * time.Millisecond)
	for _, value := range a.GetProcessMetrics() {
		if !value.CPUSampled || value.CPUPercent < 0 || value.CPUPercent > 100 {
			t.Fatalf("CPU sample: %+v", value)
		}
	}
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	waitStatus(t, a, frontendID(p.ID), "stopped")
	if len(a.GetProcessMetrics()) != 0 {
		t.Fatal("stopped process metrics retained")
	}
}
func TestCPUUsesAllLogicalCoresAndResetsForReplacementRuns(t *testing.T) {
	r := &run{}
	at := time.Now()
	previous := cpuSample{r, 0, at.Add(-time.Second)}
	value, sampled := normalizedCPU(previous, r, 1e7, at, 4)
	if !sampled || value != 25 {
		t.Fatalf("CPU normalization: %v %v", value, sampled)
	}
	if _, sampled := normalizedCPU(previous, &run{}, 1e7, at, 4); sampled {
		t.Fatal("replacement inherited old sample")
	}
}
func TestDesktopClosePolicyAndPreferences(t *testing.T) {
	a := NewApp()
	a.configPath = filepath.Join(t.TempDir(), "projects.json")
	if a.GetDesktopSettings().CloseToTray {
		t.Fatal("default no longer exits")
	}
	if err := a.SaveDesktopSettings(DesktopSettings{CloseToTray: true, TrayAvailable: true}); err != nil {
		t.Fatal(err)
	}
	reopened := NewApp()
	reopened.configPath = a.configPath
	reopened.loadDesktopSettings()
	settings := reopened.GetDesktopSettings()
	if !settings.CloseToTray || settings.TrayAvailable {
		t.Fatal("persisted runtime tray state")
	}
	if closeShouldHide(settings, false) {
		t.Fatal("hid without a recovery icon")
	}
	settings.TrayAvailable = true
	if !closeShouldHide(settings, false) || closeShouldHide(settings, true) {
		t.Fatal("tray quit was prevented")
	}
	if err := a.HideToTray(); err == nil {
		t.Fatal("headless app hid window")
	}
}
func TestNativeTrayRegistersAndRemovesIcon(t *testing.T) {
	tray := newWindowsTray(nil, nil, nil)
	if err := tray.start(); err != nil {
		t.Skipf("desktop shell unavailable: %v", err)
	}
	if !tray.available.Load() || tray.window.Load() == 0 {
		t.Fatal("tray not registered")
	}
	tray.close()
	select {
	case <-tray.done:
	case <-time.After(time.Second):
		t.Fatal("tray window did not exit")
	}
	if tray.available.Load() || tray.window.Load() != 0 {
		t.Fatal("tray state retained")
	}
}
func TestReleaseComparisonAndSafeReleaseLink(t *testing.T) {
	for _, item := range []struct {
		current, latest   string
		newer, comparable bool
	}{
		{"v0.1.1", "v0.1.2", true, true}, {"v0.2.0", "v0.1.9", false, true},
		{"v0.1.2-beta.1", "v0.1.2", true, true}, {"v0.1.2", "v0.1.2", false, true},
		{"dev", "v0.1.2", false, false}, {"v0.1.2", "v0.1.3-beta", false, false},
	} {
		got, known := newerRelease(item.current, item.latest)
		if got != item.newer || known != item.comparable {
			t.Fatalf("version: %+v", item)
		}
	}
	result, err := makeReleaseCheck(BuildInfo{Version: "v0.1.1"}, []byte(`{"tag_name":"v0.1.2","html_url":"https://evil.example/","published_at":"2026-10-08T00:00:00Z"}`))
	if err != nil || !result.HasUpdate || result.ReleaseURL != releasesURL+"/tag/v0.1.2" {
		t.Fatalf("release: %+v %v", result, err)
	}
	for _, data := range []string{`{"tag_name":"v1.2.3","draft":true}`, `{"tag_name":"v1.2.3","prerelease":true}`, `{"tag_name":"v1.2.3/evil"}`, "invalid"} {
		if _, err := makeReleaseCheck(BuildInfo{Version: "v0.1.1"}, []byte(data)); err == nil {
			t.Fatalf("accepted release: %s", data)
		}
	}
}
func TestReleaseCheckUsesExistingCLIWithoutPersonalTokenConfiguration(t *testing.T) {
	dir := t.TempDir()
	exe, _ := os.Executable()
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "gh.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("RUNNER_TEST_HELPER", "1")
	t.Setenv("RUNNER_TEST_UPDATE", `{"tag_name":"v0.1.2","draft":false,"prerelease":false}`)
	a := NewApp()
	result, err := a.CheckForUpdates()
	if err != nil || result.LatestVersion != "v0.1.2" {
		t.Fatalf("CLI release: %+v %v", result, err)
	}
	t.Setenv("RUNNER_TEST_UPDATE", "timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := fetchLatestRelease(ctx); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("update timeout: %v", err)
	}
}
