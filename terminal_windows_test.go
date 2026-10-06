//go:build windows

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

var terminalEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

func terminalTestApp(t *testing.T) *App {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "终端 工作树")
	front := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(front, 0700); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.configPath = filepath.Join(t.TempDir(), "projects.json")
	a.projects["test"] = Project{ID: "test", Name: "terminal test", Directory: dir, Kind: "spring-maven", Port: 8080,
		Environment: map[string]string{"TERMINAL_TEST_CONTEXT": dir, "TERMINAL_TEST_VALUE": "后端"},
		Frontend: &FrontendConfig{Directory: front, Port: 82, Script: "dev", PortMode: "vite", AutoProxy: true,
			Environment: map[string]string{"TERMINAL_TEST_CONTEXT": front, "TERMINAL_TEST_VALUE": "前端"}}}
	t.Cleanup(func() { a.shutdown(nil) })
	return a
}

func waitTerminalText(t *testing.T, a *App, id, expected string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var text string
	for time.Now().Before(deadline) {
		snapshot, err := a.GetTerminalOutput(id)
		if err != nil {
			t.Fatal(err)
		}
		bytes, err := base64.StdEncoding.DecodeString(snapshot.Data)
		if err != nil {
			t.Fatal(err)
		}
		text = terminalEscape.ReplaceAllString(string(bytes), "")
		if strings.Contains(text, expected) {
			return text
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("terminal did not output %q: %q", expected, text)
	return ""
}

func terminalCommand(t *testing.T, a *App, id, command string) {
	t.Helper()
	if err := a.WriteTerminal(id, command+"\r"); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalProjectContextIndependentTabsAndUTF8(t *testing.T) {
	a := terminalTestApp(t)
	first, err := a.NewTerminal("test", 160, 24)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.NewTerminal("test:frontend", 160, 24)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || second.ProjectID != "test" || second.ServiceID != "test:frontend" || first.Directory == second.Directory {
		t.Fatalf("invalid tabs: %+v %+v", first, second)
	}
	for _, info := range []TerminalInfo{first, second} {
		terminalCommand(t, a, info.ID, "Write-Output ('DIRECTORY'+'='+((Get-Location).Path -eq $env:TERMINAL_TEST_CONTEXT)); Write-Output ('VALUE'+'='+$env:TERMINAL_TEST_VALUE)")
		waitTerminalText(t, a, info.ID, "DIRECTORY=True")
	}
	waitTerminalText(t, a, first.ID, "VALUE=后端")
	waitTerminalText(t, a, second.ID, "VALUE=前端")
	terminalCommand(t, a, second.ID, "Write-Output ('PROXY'+'='+$env:VUE_APP_BASE_API_TARGET)")
	waitTerminalText(t, a, second.ID, "PROXY=http://localhost:8080")
	terminalCommand(t, a, first.ID, "$terminalLocal = 'first'; Write-Output ('中文'+'可用')")
	waitTerminalText(t, a, first.ID, "中文可用")
	terminalCommand(t, a, second.ID, "Write-Output ('ISOLATED'+'='+[string]::IsNullOrEmpty($terminalLocal))")
	waitTerminalText(t, a, second.ID, "ISOLATED=True")
	if err := a.ResizeTerminal(first.ID, 120, 30); err != nil {
		t.Fatal(err)
	}
	terminalCommand(t, a, first.ID, "Write-Output ('SIZE'+'='+$Host.UI.RawUI.WindowSize.Width)")
	waitTerminalText(t, a, first.ID, "SIZE=120")
	if err := a.CloseTerminal(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetTerminalOutput(first.ID); err == nil {
		t.Fatal("closed tab retained")
	}
	terminalCommand(t, a, second.ID, "Write-Output ('STILL'+'ALIVE')")
	waitTerminalText(t, a, second.ID, "STILLALIVE")
}

func TestTerminalCtrlCAndClosingKillsChildProcess(t *testing.T) {
	a := terminalTestApp(t)
	info, err := a.NewTerminal("test", 160, 24)
	if err != nil {
		t.Fatal(err)
	}
	terminalCommand(t, a, info.ID, "Write-Output ('WAIT'+'ING'); Start-Sleep -Seconds 120; Write-Output ('SHOULD'+'NOTCOMPLETE')")
	waitTerminalText(t, a, info.ID, "WAITING")
	if err := a.WriteTerminal(info.ID, "\x03"); err != nil {
		t.Fatal(err)
	}
	// PowerShell flushes queued input when handling Ctrl+C. Wait for its new
	// prompt before typing the next command, just as a user does interactively.
	waitTerminalText(t, a, info.ID, "WAITING\r\nPS ")
	terminalCommand(t, a, info.ID, "Write-Output ('INTERRUPT'+'ED')")
	text := waitTerminalText(t, a, info.ID, "INTERRUPTED")
	if strings.Contains(text, "SHOULDNOTCOMPLETE") {
		t.Fatal("Ctrl+C did not cancel the command")
	}
	terminalCommand(t, a, info.ID, "$child = Start-Process -FilePath ($env:SystemRoot+'\\System32\\ping.exe') -ArgumentList '127.0.0.1','-t' -NoNewWindow -PassThru; Write-Output ('CHILD'+'PID='+$child.Id)")
	text = waitTerminalText(t, a, info.ID, "CHILDPID=")
	match := regexp.MustCompile(`CHILDPID=(\d+)`).FindStringSubmatch(text)
	if len(match) != 2 {
		t.Fatalf("child PID missing: %q", text)
	}
	pid, _ := strconv.Atoi(match[1])
	child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	if err := a.CloseTerminal(info.ID); err != nil {
		t.Fatal(err)
	}
	if state, err := windows.WaitForSingleObject(child, 3000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("terminal child survived closure: state=%d err=%v", state, err)
	}
}

func TestTerminalNaturalExitDeleteAndShutdownCleanup(t *testing.T) {
	a := terminalTestApp(t)
	info, err := a.NewTerminal("test", 120, 24)
	if err != nil {
		t.Fatal(err)
	}
	session, _ := a.terminal(info.ID)
	terminalCommand(t, a, info.ID, "exit 23")
	select {
	case <-session.done:
	case <-time.After(15 * time.Second):
		t.Fatal("shell exit never completed")
	}
	snapshot, err := a.GetTerminalOutput(info.ID)
	if err != nil || snapshot.Info.State != "exited" || snapshot.Info.ExitCode == nil || *snapshot.Info.ExitCode != 23 {
		t.Fatalf("exit state lost: %+v %v", snapshot.Info, err)
	}
	if _, err := a.NewTerminal("test:frontend", 120, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteProject("test"); err != nil {
		t.Fatal(err)
	}
	if len(a.GetTerminals()) != 0 {
		t.Fatal("deleting config left terminal sessions")
	}
	a.projects["test"] = Project{ID: "test", Directory: t.TempDir()}
	last, err := a.NewTerminal("test", 120, 24)
	if err != nil {
		t.Fatal(err)
	}
	lastSession, _ := a.terminal(last.ID)
	a.shutdown(nil)
	select {
	case <-lastSession.done:
	default:
		t.Fatal("shutdown left terminal alive")
	}
	if len(a.GetTerminals()) != 0 {
		t.Fatal("shutdown retained terminal sessions")
	}
	if _, err := a.NewTerminal("test", 120, 24); err == nil {
		t.Fatal("terminal opened during shutdown")
	}
}

func TestTerminalWindowsPowerShellFallback(t *testing.T) {
	a := terminalTestApp(t)
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	p := a.projects["test"]
	p.Environment["PATH"] = system
	a.projects["test"] = p
	info, err := a.NewTerminal("test", 160, 24)
	if err != nil {
		t.Fatal(err)
	}
	if info.Shell != "PowerShell" {
		t.Fatalf("fallback not selected: %s", info.Shell)
	}
	terminalCommand(t, a, info.ID, "Write-Output ('系统'+'终端可用')")
	waitTerminalText(t, a, info.ID, "系统终端可用")
}

func TestTerminalFailedProcessCreationReleasesPipes(t *testing.T) {
	completed := make(chan error, 1)
	directory := t.TempDir()
	shell := filepath.Join(t.TempDir(), "missing-shell.exe")
	go func() {
		_, err := startTerminal(TerminalInfo{ID: "failed", Directory: directory},
			shell, os.Environ(), windows.Coord{X: 80, Y: 24},
			func(TerminalOutput) {}, func(TerminalInfo) {})
		completed <- err
	}()
	select {
	case err := <-completed:
		if err == nil {
			t.Fatal("nonexistent shell started")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed process creation hung while closing pipes")
	}
}
