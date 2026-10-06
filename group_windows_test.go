//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func pairedFixture(t *testing.T) (Project, *App) {
	t.Helper()
	p, root := mavenFixture(t)
	if err := os.WriteFile(p.ConfigFile, []byte("# local fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ui := filepath.Join(root, "ruoyi-ui")
	nodeDir := filepath.Join(root, "Node 23")
	for _, dir := range []string{ui, nodeDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(p.Environment["RUNNER_TEST_JAVA"])
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		filepath.Join(nodeDir, "node.exe"): data,
		filepath.Join(nodeDir, "npm.cmd"):  []byte("@echo off\r\nnode.exe frontend-helper %*\r\n"),
		filepath.Join(ui, "package.json"):  []byte(`{"scripts":{"dev":"vue-cli-service serve","dev:vite":"vite"}}`),
	} {
		if err := os.WriteFile(name, content, 0700); err != nil {
			t.Fatal(err)
		}
	}
	port := freePort(t)
	for port == p.Port {
		port = freePort(t)
	}
	p.Environment["BACKEND_ONLY"] = "backend-value"
	p.Frontend = &FrontendConfig{Directory: ui, Port: port, PackageManager: "npm", Script: "dev:vite", PortMode: "vite", NodeHome: nodeDir, ToolPath: filepath.Join(nodeDir, "npm.cmd"), AutoProxy: true,
		Environment: map[string]string{"RUNNER_TEST_HELPER": "1", "RUNNER_TEST_PORT": strconv.Itoa(port), "RUNNER_TEST_RUN_ARGS": filepath.Join(root, "frontend-args.json"), "RUNNER_TEST_ENV": filepath.Join(root, "frontend-env.json"), "FRONTEND_ONLY": "frontend-value"}}
	a := NewApp()
	a.configPath = filepath.Join(root, "projects.json")
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return p, a
}

func TestPairedGroupStartRepairRestartStop(t *testing.T) {
	p, a := pairedFixture(t)
	child := frontendID(p.ID)
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	backend := waitStatus(t, a, p.ID, "running")
	frontend := waitStatus(t, a, child, "running")
	if len(a.ListProjects()) != 1 || len(a.GetStatuses()) != 2 {
		t.Fatal("pair must remain one saved project with two services")
	}
	data, err := os.ReadFile(p.Frontend.Environment["RUNNER_TEST_ENV"])
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	if env["VUE_APP_BASE_API_TARGET"] != "http://localhost:"+strconv.Itoa(p.Port) || env["port"] != strconv.Itoa(p.Frontend.Port) || env["BACKEND_ONLY"] != "" || env["FRONTEND_ONLY"] != "frontend-value" {
		t.Fatalf("wrong frontend environment: %v", env)
	}
	data, err = os.ReadFile(p.Frontend.Environment["RUNNER_TEST_RUN_ARGS"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"run","dev:vite","--","--port"`) || !strings.Contains(string(data), `"--strictPort"`) {
		t.Fatalf("wrong frontend command: %s", data)
	}
	for _, line := range a.GetLogs(p.ID) {
		if strings.Contains(line.Text, "frontend fixture") {
			t.Fatal("frontend output leaked into backend log")
		}
	}
	for _, line := range a.GetLogs(child) {
		if strings.Contains(line.Text, "backend fixture") || strings.Contains(line.Text, "构建当前工作树") {
			t.Fatal("backend output leaked into frontend log")
		}
	}
	if err := a.StopService(child); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, child, "stopped")
	if got := waitStatus(t, a, p.ID, "running"); got.PID != backend.PID {
		t.Fatal("stopping frontend also restarted backend")
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, child, "running")
	if got := waitStatus(t, a, p.ID, "running"); got.PID != backend.PID {
		t.Fatal("repairing group restarted existing backend")
	}
	if err := a.RestartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if got := waitStatus(t, a, p.ID, "running"); got.PID == backend.PID {
		t.Fatal("backend was not restarted")
	}
	if got := waitStatus(t, a, child, "running"); got.PID == frontend.PID {
		t.Fatal("frontend was not restarted")
	}
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	waitStatus(t, a, child, "stopped")
	if canConnect(p.Port) || canConnect(p.Frontend.Port) {
		t.Fatal("group stop left a child process serving")
	}
}

func TestFrontendFailureKeepsBackendAvailable(t *testing.T) {
	p, a := pairedFixture(t)
	p.Frontend.Environment["RUNNER_TEST_FRONTEND_FAIL"] = "1"
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	waitStatus(t, a, frontendID(p.ID), "failed")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
}

func TestGroupPortConflictStartsNeitherService(t *testing.T) {
	p, a := pairedFixture(t)
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(p.Frontend.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := a.StartProject(p.ID); err == nil {
		t.Fatal("expected frontend port conflict")
	}
	if isRunning(a, p.ID) || isRunning(a, frontendID(p.ID)) {
		t.Fatal("port preflight launched part of the group")
	}
	if _, err := os.Stat(p.Environment["RUNNER_TEST_BUILD_ARGS"]); !os.IsNotExist(err) {
		t.Fatal("backend build unexpectedly started")
	}
}

func TestFrontendOnlyRunLocksGroupConfiguration(t *testing.T) {
	p, a := pairedFixture(t)
	if err := a.StartService(frontendID(p.ID)); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "running")
	if _, err := a.SaveProject(p); err == nil {
		t.Fatal("saved config while frontend running")
	}
	if err := a.DeleteProject(p.ID); err == nil {
		t.Fatal("deleted group while frontend running")
	}
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "stopped")
	if err := a.DeleteProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.GetLogs(frontendID(p.ID))) != 0 {
		t.Fatal("deleted frontend logs retained")
	}
}

func TestGroupStopDuringBuildKillsBothProcessTrees(t *testing.T) {
	p, a := pairedFixture(t)
	p.Environment["RUNNER_TEST_BUILD"] = "wait"
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, frontendID(p.ID), "running")
	waitStatus(t, a, p.ID, "building")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	waitStatus(t, a, frontendID(p.ID), "stopped")
	if canConnect(p.Port) || canConnect(p.Frontend.Port) {
		t.Fatal("group cancellation leaked a process")
	}
	if _, err := os.Stat(p.Environment["RUNNER_TEST_RUN_ARGS"]); !os.IsNotExist(err) {
		t.Fatal("cancelled backend launched application")
	}
}

func TestDetectPairedFrontendPrefersViteWithinCheckout(t *testing.T) {
	p, a := pairedFixture(t)
	for _, directory := range []string{p.Directory, filepath.Join(p.Directory, "app"), p.Frontend.Directory} {
		f, err := a.DetectFrontend(directory)
		if err != nil {
			t.Fatal(err)
		}
		if f.Directory != p.Frontend.Directory || f.Script != "dev:vite" || f.PortMode != "vite" || !f.AutoProxy {
			t.Fatalf("wrong detection: %+v", f)
		}
	}
	if _, err := a.DetectFrontend(""); err == nil {
		t.Fatal("empty path should not inspect current working directory")
	}
	p.Frontend.Script = "missing"
	if _, err := a.SaveProject(p); err == nil {
		t.Fatal("accepted nonexistent frontend script")
	}
	p.Frontend.Script = "dev:vite"
	p.Frontend.Port = p.Port
	if _, err := a.SaveProject(p); err == nil {
		t.Fatal("accepted equal frontend and backend ports")
	}
}

func TestPairedConfigurationPersistsProxyAndNodeSettings(t *testing.T) {
	p, a := pairedFixture(t)
	t.Setenv("PROJECT_RUNNER_CONFIG", a.configPath)
	reopened := NewApp()
	reopened.startup(nil)
	items := reopened.ListProjects()
	if len(items) != 1 || items[0].Frontend == nil || items[0].Frontend.NodeHome != p.Frontend.NodeHome || items[0].Frontend.Script != "dev:vite" {
		t.Fatalf("pair settings lost: %+v", items)
	}
	manual := *p.Frontend
	manual.AutoProxy = false
	manual.Environment = map[string]string{"VUE_APP_BASE_API_TARGET": "http://custom:9090", "port": "wrong", "PORT": "also-wrong", "custom_api_proxy": "http://stale:9090"}
	p.Frontend = &manual
	derived := frontendProject(p)
	if derived.Environment["VUE_APP_BASE_API_TARGET"] != "http://custom:9090" || derived.Environment["port"] != strconv.Itoa(manual.Port) {
		t.Fatal("manual proxy or managed port overridden incorrectly")
	}
	manual.AutoProxy = true
	manual.ProxyVariable = "CUSTOM_API_PROXY"
	derived = frontendProject(p)
	if derived.Environment["CUSTOM_API_PROXY"] != "http://localhost:"+strconv.Itoa(p.Port) {
		t.Fatal("custom proxy variable not passed")
	}
	if manual.Environment["port"] != "wrong" {
		t.Fatal("derived runtime mutated saved environment")
	}
	if _, ok := derived.Environment["PORT"]; ok {
		t.Fatal("uppercase alias can override managed port on Windows")
	}
	if _, ok := derived.Environment["custom_api_proxy"]; ok {
		t.Fatal("lowercase alias can override managed proxy on Windows")
	}
	// Closing prevents a concurrent/new group request from leaking processes.
	a.shutdown(nil)
	if err := a.StartProject(p.ID); err == nil {
		t.Fatal("start allowed after shutdown")
	}
}
