//go:build windows

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreflightAggregatesBothServicesWithoutStartingThem(t *testing.T) {
	p, a := pairedFixture(t)
	p.ConfigFile = filepath.Join(p.Directory, "missing.properties")
	p.Frontend.Script = "missing-script"
	if err := os.WriteFile(filepath.Join(p.Frontend.Directory, "package.json"), []byte(`{"scripts":{"dev":"vite"},"dependencies":{"vite":"1"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	p.Frontend.Port = listener.Addr().(*net.TCPAddr).Port
	report := a.checkServices([]Project{p, frontendProject(p)}, false)
	if report.Allowed {
		t.Fatal("invalid pair allowed")
	}
	labels := map[string]bool{}
	for _, check := range report.Checks {
		if check.State == "error" {
			labels[check.Label] = true
		}
		if check.Label == "端口" && len(check.Owners) != 0 {
			if check.Owners[0].PID != uint32(os.Getpid()) || check.Owners[0].ServiceID != "" {
				t.Fatalf("external process misidentified: %+v", check.Owners)
			}
		}
	}
	for _, label := range []string{"外部配置", "启动脚本", "前端依赖", "端口"} {
		if !labels[label] {
			t.Fatalf("missing aggregate error %s: %+v", label, report)
		}
	}
	if len(a.runs) != 0 || exists(p.Environment["RUNNER_TEST_BUILD_ARGS"]) || exists(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("preflight launched a build or service")
	}
	// Automatic group start rejects the entire pair before spawning either side.
	a.projects[p.ID] = p
	if err := a.StartProject(p.ID); err == nil || len(a.runs) != 0 {
		t.Fatal("invalid group partially launched")
	}
}

func TestPortOwnerFindsManagedDescendantAndStopsOnlyItsService(t *testing.T) {
	p, a := pairedFixture(t)
	id := frontendID(p.ID)
	if err := a.StartService(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, id, "running")
	owners := a.portOwners(p.Frontend.Port)
	if len(owners) != 1 || owners[0].ServiceID != id || owners[0].ServiceName == "" || owners[0].PID == 0 {
		t.Fatalf("managed descendant not identified: %+v", owners)
	}
	if err := a.StopService(owners[0].ServiceID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, id, "stopped")
	if canConnect(p.Frontend.Port) || len(a.portOwners(p.Frontend.Port)) != 0 {
		t.Fatal("managed listener survived stop")
	}
}

func TestHealthValidationAndHTTPReadiness(t *testing.T) {
	for _, value := range []string{"https://example.com/health", "file:///a", "http://user:pass@localhost/health", "http://localhost/#secret"} {
		if _, err := validateHealth(&HealthConfig{URL: value}); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	for _, h := range []*HealthConfig{{TimeoutSeconds: 4}, {TimeoutSeconds: 601}, {ExpectedStatus: 500}} {
		if _, err := validateHealth(h); err == nil {
			t.Fatalf("invalid limits accepted: %+v", h)
		}
	}
	h, err := validateHealth(&HealthConfig{URL: " http://[::1]/health "})
	if err != nil || h.TimeoutSeconds != 90 || h.ExpectedStatus != 200 {
		t.Fatalf("defaults: %+v %v", h, err)
	}
	var remoteCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remoteCalls.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, 302)
			return
		}
		if !healthy.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	p := Project{Health: &HealthConfig{URL: server.URL, ExpectedStatus: 204}}
	if ready, _ := readyProbe(context.Background(), p); ready {
		t.Fatal("unhealthy HTTP became ready just because port listens")
	}
	healthy.Store(true)
	if ready, _ := readyProbe(context.Background(), p); !ready {
		t.Fatal("expected status not accepted")
	}
	p.Health.URL = server.URL + "/redirect"
	if ready, _ := readyProbe(context.Background(), p); ready || remoteCalls.Load() != 0 {
		t.Fatal("health redirect followed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ready, _ := readyProbe(ctx, p); ready {
		t.Fatal("canceled probe succeeded")
	}
}

func TestRestartPreflightDoesNotStopExistingServiceOnInvalidSettings(t *testing.T) {
	p, a := pairedFixture(t)
	id := frontendID(p.ID)
	if err := a.StartService(id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, id, "running")
	// A local file can disappear after the configuration was saved. Restart
	// must report that before it tears down the existing frontend.
	a.mu.Lock()
	broken := a.projects[p.ID]
	broken.ConfigFile = filepath.Join(p.Directory, "missing-config.properties")
	a.projects[p.ID] = broken
	a.mu.Unlock()
	if err := a.RestartProject(p.ID); err == nil {
		t.Fatal("restart allowed missing config")
	}
	if !canConnect(p.Frontend.Port) {
		t.Fatal("invalid restart stopped existing frontend")
	}
}

func TestReadinessTimeoutPreservesRunAndCancellationDoesNotChangeState(t *testing.T) {
	a := NewApp()
	r := &run{done: make(chan struct{}), started: time.Now()}
	r.stage.Store(1)
	a.runs["sample"] = r
	a.statuses["sample"] = Status{ID: "sample", State: "starting", StartedAt: r.started.UnixMilli()}
	p := Project{ID: "sample", Port: freePort(t), Health: &HealthConfig{TimeoutSeconds: 5}}
	ended := make(chan struct{})
	done := make(chan struct{})
	defer close(ended)
	go func() { a.waitForProjectReady(p.ID, p, r, ended, 1); close(done) }()
	waitStatus(t, a, p.ID, "unready")
	status := a.GetStatuses()[0]
	if status.State != "unready" || !strings.Contains(status.Error, "进程仍在运行") || a.runs[p.ID] != r {
		t.Fatalf("timeout lost process state: %+v", status)
	}
	// A delayed listener recovers readiness without restarting or duplicating
	// the process, and clears the earlier timeout message.
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	status = waitStatus(t, a, p.ID, "running")
	if status.Error != "" {
		t.Fatalf("stale timeout after ready: %+v", status)
	}
	<-done
	a.statuses[p.ID] = Status{ID: p.ID, State: "starting"}
	canceledStage := make(chan struct{})
	close(canceledStage)
	a.waitForProjectReady(p.ID, p, r, canceledStage, 1)
	if a.GetStatuses()[0].State != "starting" {
		t.Fatal("canceled stage changed readiness")
	}
}

func TestAutoOpenWaitsForEntireGroupAndOpensOncePerRun(t *testing.T) {
	a := NewApp()
	p := Project{ID: "one", Port: 8081, AutoOpen: true, Frontend: &FrontendConfig{Port: 82, PortMode: "vite"}}
	a.projects[p.ID] = p
	a.statuses[p.ID] = Status{State: "starting", StartedAt: 1}
	a.statuses[frontendID(p.ID)] = Status{State: "running", StartedAt: 2}
	if a.takeReadyPage(frontendID(p.ID)) != "" {
		t.Fatal("opened before backend ready")
	}
	a.statuses[p.ID] = Status{State: "running", StartedAt: 1}
	if got := a.takeReadyPage(p.ID); got != "http://127.0.0.1:82/" {
		t.Fatalf("wrong ready page %q", got)
	}
	if a.takeReadyPage(frontendID(p.ID)) != "" {
		t.Fatal("duplicate ready event reopened browser")
	}
	a.statuses[p.ID] = Status{State: "unready", StartedAt: 3}
	if a.takeReadyPage(p.ID) != "" {
		t.Fatal("opened timed-out group")
	}
	a.statuses[p.ID] = Status{State: "running", StartedAt: 3}
	if a.takeReadyPage(p.ID) == "" {
		t.Fatal("new run did not open")
	}
	p.AutoOpen = false
	a.projects[p.ID] = p
	if a.takeReadyPage(p.ID) != "" {
		t.Fatal("opened after opt-out")
	}
}

func TestPreflightToolVersionUsesConfiguredEnvironment(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tool.cmd")
	if err := os.WriteFile(path, []byte("@echo off\r\necho Apache Maven %TEST_VERSION%\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := probeToolVersion(commandSpec{Executable: path, Args: []string{"--version"}, Directory: root, Env: append(os.Environ(), "TEST_VERSION=3.9.9")})
	if !strings.Contains(got, "3.9.9") {
		t.Fatalf("wrong version %q", got)
	}
	// A draft with an invalid health address fails before it can be saved/launched.
	report := NewApp().checkServices([]Project{{Kind: "node", Directory: root, PortMode: "none", Health: &HealthConfig{URL: "https://example.com"}}}, false)
	for _, check := range report.Checks {
		if check.Label == "就绪检查" && check.State == "error" {
			return
		}
	}
	t.Fatal("invalid draft health configuration omitted from aggregate")
}

func TestIPv6PortOwner(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 unavailable: " + err.Error())
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	for _, owner := range NewApp().portOwners(port) {
		if owner.PID == uint32(os.Getpid()) && owner.Address == "IPv6" {
			return
		}
	}
	t.Fatal("IPv6 listener not found at port " + strconv.Itoa(port))
}
