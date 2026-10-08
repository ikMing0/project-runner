//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeWorkflowFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func workflowStatus(a *App, id string) Status {
	for _, status := range a.GetStatuses() {
		if status.ID == id {
			return status
		}
	}
	return Status{}
}

func TestSourceSnapshotIncludesDependencyModulesAndIgnoresGeneratedFiles(t *testing.T) {
	p, root := mavenFixture(t)
	path := filepath.Join(root, "common", "src", "main", "java", "Sample.java")
	writeWorkflowFile(t, path, "class A {}")
	resource := filepath.Join(root, "app", "src", "main", "resources", "mapper.xml")
	writeWorkflowFile(t, resource, "<mapper/>")
	before, err := snapshotProjectSources(p, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"common/target/generated.java", "app/src/test/java/Test.java", "ruoyi-ui/src/index.ts", ".git/logs/HEAD"} {
		if strings.HasPrefix(name, ".git/") {
			continue
		} // fixture uses a .git worktree marker file
		writeWorkflowFile(t, filepath.Join(root, name), "ignored")
	}
	current, err := snapshotProjectSources(p, before, false)
	if err != nil || changedSourceMessage(before, current) != "" {
		t.Fatalf("ignored edits: %v", err)
	}
	info, _ := os.Stat(path)
	writeWorkflowFile(t, path, "class B {}")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	current, err = snapshotProjectSources(p, current, true)
	if err != nil || !strings.Contains(changedSourceMessage(before, current), "Sample.java") {
		t.Fatalf("same size/timestamp edit missed: %v", err)
	}
	if err := os.Remove(resource); err != nil {
		t.Fatal(err)
	}
	writeWorkflowFile(t, filepath.Join(root, "common/src/main/resources/added.xml"), "<new/>")
	current, err = snapshotProjectSources(p, current, false)
	if err != nil || !strings.HasPrefix(changedSourceMessage(before, current), "3 个") {
		t.Fatalf("add/remove: %v, %s", err, changedSourceMessage(before, current))
	}
	writeWorkflowFile(t, path, "class A {}")
	writeWorkflowFile(t, resource, "<mapper/>")
	if err := os.Remove(filepath.Join(root, "common/src/main/resources/added.xml")); err != nil {
		t.Fatal(err)
	}
	current, err = snapshotProjectSources(p, current, true)
	if err != nil || changedSourceMessage(before, current) != "" {
		t.Fatalf("revert not cleared: %v", err)
	}
}

func TestGradleSourceSnapshotSkipsFrontendAndOutput(t *testing.T) {
	root := t.TempDir()
	for path, data := range map[string]string{
		"build.gradle": "", "backend/src/main/java/Sample.java": "old",
		"frontend/package.json": "{}", "frontend/src/main/index.ts": "ignored",
		"backend/build/generated/build.gradle": "ignored", "backend/src/test/java/Test.java": "ignored",
	} {
		writeWorkflowFile(t, filepath.Join(root, path), data)
	}
	p := Project{Directory: root, Kind: "spring-gradle"}
	before, err := snapshotProjectSources(p, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Files) != 2 {
		t.Fatalf("unexpected inputs: %+v", before.Files)
	}
	// Use a size change: rapid writes can share a timestamp on Windows CI.
	writeWorkflowFile(t, filepath.Join(root, "backend/src/main/java/Sample.java"), "updated source")
	now, err := snapshotProjectSources(p, before, false)
	if err != nil || changedSourceMessage(before, now) == "" {
		t.Fatalf("Gradle module change missed: %v", err)
	}
}

func TestRunningSourceChangeCanRestartBackendWithoutStoppingFrontend(t *testing.T) {
	p, a := pairedFixture(t)
	path := filepath.Join(p.Directory, "common/src/main/java/Sample.java")
	writeWorkflowFile(t, path, "class A {}")
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	front := waitStatus(t, a, frontendID(p.ID), "running")
	writeWorkflowFile(t, path, "class B {}")
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && !workflowStatus(a, p.ID).SourceChanged {
		time.Sleep(30 * time.Millisecond)
	}
	if !workflowStatus(a, p.ID).SourceChanged {
		t.Fatal("no live source warning")
	}
	if err := a.RestartService(p.ID); err != nil {
		t.Fatal(err)
	}
	status := waitStatus(t, a, p.ID, "running")
	if status.SourceChanged || status.SourceError != "" {
		t.Fatalf("fresh run kept warning: %+v", status)
	}
	if got := workflowStatus(a, frontendID(p.ID)); got.PID != front.PID || got.StartedAt != front.StartedAt {
		t.Fatalf("frontend was restarted: %+v -> %+v", front, got)
	}
	old := &run{done: make(chan struct{})}
	old.stage.Store(1)
	a.setSourceState(p.ID, old, 1, "obsolete watcher", "")
	if workflowStatus(a, p.ID).SourceChanged {
		t.Fatal("old generation overwrote fresh run")
	}
}

func TestDependenciesProbeAndRequiredStartupGate(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	p, a := pairedFixture(t)
	p.Dependencies = []DependencyCheck{
		{Name: "MySQL", Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Required: true},
		{Name: "Redis", Host: "127.0.0.1", Port: freePort(t), Required: true},
	}
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	report := a.checkServices([]Project{p, frontendProject(p)}, false)
	if report.Allowed {
		t.Fatalf("required dependency report: %+v", report)
	}
	foundOK := false
	for _, check := range report.Checks {
		if check.Label == "依赖 · MySQL" && check.State == "ok" {
			foundOK = true
		}
	}
	if !foundOK {
		t.Fatal("successful TCP endpoint absent from report")
	}
	if err := a.StartProject(p.ID); err == nil || !strings.Contains(err.Error(), "Redis") {
		t.Fatalf("missing required dependency not blocked: %v", err)
	}
	if len(a.runs) != 0 || exists(p.Environment["RUNNER_TEST_BUILD_ARGS"]) {
		t.Fatal("preflight launched a process")
	}
	p.Dependencies[1].Required = false
	if _, err := a.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	waitStatus(t, a, frontendID(p.ID), "running")
	running := workflowStatus(a, p.ID)
	listener.Close()
	if err := a.RestartService(p.ID); err == nil {
		t.Fatal("restart ignored missing required dependency")
	}
	if got := workflowStatus(a, p.ID); got.PID != running.PID || got.StartedAt != running.StartedAt {
		t.Fatal("failed dependency check stopped the existing backend")
	}
	data, err := os.ReadFile(a.configPath)
	if err != nil || !strings.Contains(string(data), "dependencies") {
		t.Fatalf("dependency persistence: %v", err)
	}
	template, err := templateFromProject(p, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(template.Project.Dependencies) != 2 || len(shareTemplate(template).Project.Dependencies) != 0 {
		t.Fatal("local/share dependency policy")
	}
}

func TestDependenciesRejectCredentialsAndMalformedTargets(t *testing.T) {
	for _, d := range []DependencyCheck{
		{Name: "db", Host: "https://user:password@localhost", Port: 3306},
		{Name: "db", Host: "local host", Port: 3306},
		{Name: "db", Host: "localhost:3306", Port: 3306},
		{Name: "db", Host: "127.0.0.1", Port: 65536},
		{Name: "", Host: "localhost", Port: 3306},
	} {
		if _, err := validateDependencies([]DependencyCheck{d}); err == nil {
			t.Fatalf("accepted: %+v", d)
		}
	}
	if got, err := validateDependencies([]DependencyCheck{{Name: "IPv6", Host: "[::1]", Port: 6379}}); err != nil || got[0].Host != "::1" {
		t.Fatalf("IPv6: %+v %v", got, err)
	}
}

func TestIDEASourceLocationAndLaunchUsesLiteralArguments(t *testing.T) {
	p, root := mavenFixture(t)
	path := filepath.Join(root, "common", "src", "main", "java", "Sample & $(literal).java")
	writeWorkflowFile(t, path, "class Sample {}")
	resolved, args, err := sourceLocation(p, "/"+filepath.ToSlash(path), 91, 21)
	if err != nil || resolved != path {
		t.Fatalf("Maven location: %s %v", resolved, err)
	}
	executable, _ := os.Executable()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "IDEA with spaces", "bin", "idea64.exe")
	if err := os.MkdirAll(filepath.Dir(launcher), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, data, 0700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(filepath.Dir(filepath.Dir(launcher)), "plugins/maven/lib/maven3/bin/mvn.cmd")
	if got := discoverIDEA(tool); got != launcher {
		t.Fatalf("bundled Maven discovery: %s", got)
	}
	a := NewApp()
	a.configPath = filepath.Join(root, "runner/projects.json")
	a.projects[p.ID] = p
	t.Setenv("RUNNER_TEST_HELPER", "1")
	record := filepath.Join(root, "editor-args.json")
	t.Setenv("RUNNER_TEST_EDITOR", record)
	if err := a.SetIDEAPath(launcher); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenSourceInIDEA(p.ID, path, 91, 21); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !exists(record) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	data, err = os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("arguments changed: %+v vs %+v", got, args)
	}
	reopened := NewApp()
	reopened.configPath = a.configPath
	settings, err := reopened.GetEditorSettings("")
	if err != nil || settings.IDEAPath != launcher {
		t.Fatalf("editor preference persistence: %+v %v", settings, err)
	}
	if err := a.SetIDEAPath(""); err != nil {
		t.Fatal(err)
	}
	settings, err = a.GetEditorSettings("")
	if err != nil || settings.IDEAPath != "" {
		t.Fatalf("clear editor preference: %+v %v", settings, err)
	}
}

func TestIDEASourceLocationRejectsFilesOutsideCurrentWorktree(t *testing.T) {
	p, root := mavenFixture(t)
	outside := filepath.Join(t.TempDir(), "External.java")
	writeWorkflowFile(t, outside, "")
	inside := filepath.Join(root, "app/src/main/java/Inside.java")
	writeWorkflowFile(t, inside, "")
	for _, path := range []string{outside, "C:relative.java", "https://example.com/X.java", filepath.Join(root, "local config.properties")} {
		if _, _, err := sourceLocation(p, path, 1, 0); err == nil {
			t.Fatalf("accepted source: %s", path)
		}
	}
	for _, position := range [][2]int{{0, 0}, {1, -1}, {10000001, 1}} {
		if _, _, err := sourceLocation(p, inside, position[0], position[1]); err == nil {
			t.Fatalf("accepted position: %+v", position)
		}
	}
	link := filepath.Join(root, "Linked.java")
	if err := os.Symlink(outside, link); err == nil {
		if _, _, err := sourceLocation(p, link, 1, 0); err == nil {
			t.Fatal("symlink escaped worktree")
		}
	}
	nested := p
	nested.Directory = filepath.Join(root, "app")
	if _, _, err := sourceLocation(nested, inside, 1, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sourceLocation(p, "src/main/java/Inside.java", 1, 0); err != nil {
		t.Fatal("module-relative compiler path:", err)
	}
	a := NewApp()
	a.configPath = filepath.Join(root, "projects.json")
	defer a.shutdown(context.Background())
	if err := a.OpenSourceInIDEA("missing", inside, 1, 0); err == nil {
		t.Fatal("unknown service accepted")
	}
}
