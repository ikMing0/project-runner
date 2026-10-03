//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeProjectStartsAndStopsWithoutOrphan(t *testing.T) {
	if _, err := os.Stat(filepath.Join(os.Getenv("APPDATA"), "npm", "npm.cmd")); err != nil {
		// npm.cmd is normally on PATH; if the test host has only Node, skip.
		if _, err := exec.LookPath("npm.cmd"); err != nil {
			t.Skip("npm.cmd unavailable")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"node server.js"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	server := `const http=require('http'); http.createServer((req,res)=>res.end('ok')).listen(Number(process.env.PORT),'127.0.0.1',()=>console.log('READY'));`
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte(server), 0600); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	a := NewApp()
	a.projects["sample"] = Project{ID: "sample", Name: "sample", Directory: dir, Kind: "node", Port: port, PackageManager: "npm", Script: "dev", PortMode: "env"}
	if err := a.StartProject("sample"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.StopProject("sample") })
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if canConnect(port) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !canConnect(port) {
		t.Fatalf("server never listened; logs: %+v", a.GetLogs("sample"))
	}
	if err := a.StopProject("sample"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !canConnect(port) {
			a.mu.Lock()
			_, active := a.runs["sample"]
			a.mu.Unlock()
			if !active {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("child process still listening after stop: %d", port)
}

func TestStartupDurationIsRecordedWhenProjectBecomesReady(t *testing.T) {
	a := NewApp()
	started := time.Now().Add(-1500 * time.Millisecond)
	r := &run{started: started}
	a.runs["sample"] = r
	a.statuses["sample"] = Status{ID: "sample", State: "starting", StartedAt: started.UnixMilli()}
	a.markRunning("sample", r)
	status := a.GetStatuses()[0]
	if status.State != "running" || status.StartupDurationMs == nil || *status.StartupDurationMs < 1500 {
		t.Fatalf("startup duration missing after ready: %+v", status)
	}
	if status.StartedAt != started.UnixMilli() {
		t.Fatalf("start time changed: %+v", status)
	}
	previous := status.StartupDurationMs
	a.markRunning("sample", r)
	a.markRunning("sample", &run{started: started})
	if got := a.GetStatuses()[0]; got.StartupDurationMs != previous {
		t.Fatalf("ready time changed after duplicate or stale event: %+v", got)
	}
}

func TestWindowShutdownStopsManagedChild(t *testing.T) {
	if _, err := exec.LookPath("npm.cmd"); err != nil {
		t.Skip("npm.cmd unavailable")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"node server.js"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	server := `require('http').createServer((req,res)=>res.end('ok')).listen(Number(process.env.PORT),'127.0.0.1')`
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte(server), 0600); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	a := NewApp()
	a.projects["shutdown"] = Project{ID: "shutdown", Name: "shutdown", Directory: dir, Kind: "node", Port: port, PackageManager: "npm", Script: "dev", PortMode: "env"}
	if err := a.StartProject("shutdown"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && !canConnect(port) {
		time.Sleep(100 * time.Millisecond)
	}
	if !canConnect(port) {
		t.Fatalf("server never listened; logs: %+v", a.GetLogs("shutdown"))
	}
	a.shutdown(nil)
	if canConnect(port) {
		t.Fatalf("server survived application shutdown on port %d", port)
	}
}

func TestSavedConfigurationCanBeReopened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv("PROJECT_RUNNER_CONFIG", path)
	dir := t.TempDir()
	a := NewApp()
	a.startup(nil)
	p, err := a.SaveProject(Project{Name: "feature-a", Directory: dir, Kind: "spring-maven", Port: 8081, ConfigProperty: "application.config.path"})
	if err != nil {
		t.Fatal(err)
	}
	p.Port = 8082
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	reopened := NewApp()
	reopened.startup(nil)
	items := reopened.ListProjects()
	if len(items) != 1 || items[0].Port != 8082 || items[0].Name != "feature-a" {
		t.Fatalf("configuration did not persist: %+v", items)
	}
}

func TestLaunchSpecPassesJavaPropertiesToApplication(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "application.properties")
	if err := os.WriteFile(config, []byte("example=true"), 0600); err != nil {
		t.Fatal(err)
	}
	p := Project{Kind: "spring-maven", Directory: dir, Port: 8081, ConfigFile: config, ConfigProperty: "application.config.path"}
	spec, err := buildCommand(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) < 2 || spec.Args[0] != "spring-boot:run" {
		t.Fatalf("wrong Maven goal: %v", spec.Args)
	}
	tool := filepath.Join(dir, "mvn.cmd")
	if err := os.WriteFile(tool, []byte("@echo off\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p.ToolPath = tool
	explicit, err := buildCommand(p)
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Executable != tool {
		t.Fatalf("explicit build tool ignored: %s", explicit.Executable)
	}
	properties := spec.Args[1]
	for _, want := range []string{"-Dserver.port=8081", "-Dapplication.config.path=" + config} {
		if !strings.Contains(properties, want) {
			t.Errorf("missing %q in %q", want, properties)
		}
	}
	gradle := p
	gradle.Kind = "spring-gradle"
	gradleSpec, err := buildCommand(gradle)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanTemporary(gradleSpec.Temporary)
	content, err := os.ReadFile(gradleSpec.Temporary[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-Dserver.port=8081", "-Dapplication.config.path="} {
		if !strings.Contains(string(content), want) {
			t.Errorf("missing %q in Gradle init script", want)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func canConnect(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
