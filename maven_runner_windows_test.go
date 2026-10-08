//go:build windows

package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Copies of the test executable stand in for a JDK. They do not load the app,
// contact a database, download dependencies, or require Java on the test host.
func TestMain(m *testing.M) {
	if os.Getenv("RUNNER_TEST_HELPER") == "1" {
		os.Exit(helperProcess())
	}
	os.Exit(m.Run())
}

func helperProcess() int {
	args := os.Args[1:]
	if response := os.Getenv("RUNNER_TEST_UPDATE"); response != "" {
		if response == "timeout" {
			time.Sleep(time.Minute)
			return 1
		}
		fmt.Print(response)
		return 0
	}
	if path := os.Getenv("RUNNER_TEST_EDITOR"); path != "" {
		data, _ := json.Marshal(args)
		if os.WriteFile(path, data, 0600) != nil {
			return 1
		}
		return 0
	}
	if os.Getenv("RUNNER_TEST_CODEX") == "1" {
		return codexHelperProcess(args)
	}
	if len(args) != 0 && args[0] == "frontend-helper" {
		values := map[string]string{}
		for _, key := range []string{"port", "PORT", "VUE_APP_BASE_API_TARGET", "FRONTEND_ONLY", "BACKEND_ONLY"} {
			values[key] = os.Getenv(key)
		}
		data, _ := json.Marshal(values)
		_ = os.WriteFile(os.Getenv("RUNNER_TEST_ENV"), data, 0600)
		if os.Getenv("RUNNER_TEST_FRONTEND_FAIL") == "1" {
			fmt.Fprintln(os.Stderr, "frontend fixture failed")
			return 11
		}
		fmt.Println("frontend fixture ready")
	}
	if len(args) != 0 && args[0] == "record" {
		data, _ := json.Marshal(args[1:])
		_ = os.WriteFile(os.Getenv("RUNNER_TEST_RECORD"), data, 0600)
		return 0
	}
	if len(args) != 0 && args[0] == "build-helper" {
		recordRecoveryStage("build", args[1:])
		data, _ := json.Marshal(args[1:])
		_ = os.WriteFile(os.Getenv("RUNNER_TEST_BUILD_ARGS"), data, 0600)
		switch os.Getenv("RUNNER_TEST_BUILD") {
		case "fail":
			fmt.Fprintln(os.Stderr, "[ERROR] Current source does not compile")
			return 7
		case "wait":
			child := exec.Command(os.Args[0], "child-service")
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Run(); err != nil {
				return 8
			}
			return 0
		}
		if slices.Contains(args, "clean") {
			_ = os.Remove(os.Getenv("RUNNER_TEST_STALE_RESOURCE"))
		}
		if err := writeExecutableJar(os.Getenv("RUNNER_TEST_JAR")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 9
		}
		return 0
	}
	if len(args) == 0 || args[0] != "child-service" {
		if len(args) == 0 || args[0] != "frontend-helper" {
			recordRecoveryStage("start", args)
			failure := os.Getenv("RUNNER_TEST_APP_ERROR")
			if stale := os.Getenv("RUNNER_TEST_STALE_RESOURCE"); stale != "" && exists(stale) {
				failure = "mapper"
			}
			if failure != "" {
				switch failure {
				case "mapper":
					fmt.Fprintln(os.Stderr, "Error parsing Mapper XML. Failed to parse mapping resource: mapper/StaleMapper.xml")
					fmt.Fprintln(os.Stderr, "Caused by: java.lang.ClassNotFoundException: Cannot find class: sample.RemovedEntity")
				case "database":
					fmt.Fprintln(os.Stderr, "Caused by: java.sql.SQLNonTransientConnectionException: Connection refused")
				case "initialization":
					fmt.Fprintln(os.Stderr, "Error parsing Mapper XML")
					fmt.Fprintln(os.Stderr, "Caused by: java.lang.NoClassDefFoundError: Could not initialize class sample.Entity")
				}
				return 13
			}
		}
		if len(args) == 0 || args[0] != "frontend-helper" {
			fmt.Println("backend fixture ready")
		}
		data, _ := json.Marshal(args)
		_ = os.WriteFile(os.Getenv("RUNNER_TEST_RUN_ARGS"), data, 0600)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("RUNNER_TEST_PORT"))
	if err != nil {
		return 10
	}
	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	return 0
}

func writeExecutableJar(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	manifest, err := archive.Create("META-INF/MANIFEST.MF")
	if err == nil {
		_, err = manifest.Write([]byte("Manifest-Version: 1.0\r\nMain-Class: sample.Main\r\n\r\n"))
	}
	closeErr := archive.Close()
	file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func mavenFixture(t *testing.T) (Project, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "工作树 A & demo")
	for _, directory := range []string{"common", "app", "JDK 21/bin", "Maven 工具 (1)"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"local config.properties": "# fixture config\n",
		"pom.xml":                 `<project><artifactId>parent</artifactId><version>1</version><packaging>pom</packaging><modules><module>common</module><module>app</module></modules></project>`,
		"common/pom.xml":          `<project><artifactId>common</artifactId><version>1</version></project>`,
		"app/pom.xml":             `<project><artifactId>app</artifactId><parent><version>1</version></parent><properties><output.name>${project.artifactId}-test</output.name></properties><build><finalName>${output.name}</finalName><plugins><plugin><artifactId>spring-boot-maven-plugin</artifactId><executions><execution><goals><goal>repackage</goal></goals></execution></executions></plugin></plugins></build></project>`,
		".git":                    "fixture",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable, _ := os.Executable()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(root, "JDK 21", "bin", "java.exe")
	if err := os.WriteFile(java, data, 0700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(root, "Maven 工具 (1)", "mvn.cmd")
	if err := os.WriteFile(tool, []byte("@echo off\r\n\"%RUNNER_TEST_JAVA%\" build-helper %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	p := Project{ID: "fixture", Name: "fixture", Kind: "spring-maven", Directory: root, Module: "app", Port: port, JavaHome: filepath.Join(root, "JDK 21"), ToolPath: tool,
		ConfigFile: filepath.Join(root, "local config.properties"), ConfigProperty: "application.config.path", JVMArgs: "-Xmx128m", AppArgs: "--example=value with spaces",
		Environment: map[string]string{
			"RUNNER_TEST_JAVA":   java,
			"RUNNER_TEST_HELPER": "1", "RUNNER_TEST_PORT": strconv.Itoa(port),
			"RUNNER_TEST_JAR":        filepath.Join(root, "app", "target", "app-test.jar"),
			"RUNNER_TEST_BUILD_ARGS": filepath.Join(root, "build-args.json"),
			"RUNNER_TEST_RUN_ARGS":   filepath.Join(root, "run-args.json"),
		}}
	return p, root
}

func waitStatus(t *testing.T, a *App, id, state string) Status {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		for _, status := range a.GetStatuses() {
			if status.ID == id && status.State == state {
				return status
			}
			if status.ID == id && status.State == "failed" && state != "failed" {
				t.Fatalf("unexpected failure: %+v logs=%+v", status, a.GetLogs(id))
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("never reached %s; statuses=%+v logs=%+v", state, a.GetStatuses(), a.GetLogs(id))
	return Status{}
}

func TestMavenBuildsReactorThenRunsCurrentJar(t *testing.T) {
	p, root := mavenFixture(t)
	a := NewApp()
	a.projects[p.ID] = p
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if isRunning(a, p.ID) {
			_ = a.StopProject(p.ID)
			waitStatus(t, a, p.ID, "stopped")
		}
	})
	status := waitStatus(t, a, p.ID, "running")
	if status.StartupDurationMs == nil {
		t.Fatal("startup duration missing")
	}
	var build, runArgs []string
	for name, target := range map[string]*[]string{"build-args.json": &build, "run-args.json": &runArgs} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	joined := strings.Join(build, " ")
	if !strings.Contains(joined, "-pl app -am clean package") || strings.Contains(joined, "install") || strings.Count(joined, "clean") != 1 {
		t.Fatalf("wrong reactor build: %v", build)
	}
	want := []string{"-Xmx128m", "-Dserver.port=" + strconv.Itoa(p.Port), "-Dapplication.config.path=" + p.ConfigFile, "-jar", p.Environment["RUNNER_TEST_JAR"], "--example=value with spaces"}
	if !reflect.DeepEqual(runArgs, want) {
		t.Fatalf("runtime args=%q want=%q", runArgs, want)
	}
	if err := a.RebuildProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	data, _ := os.ReadFile(filepath.Join(root, "build-args.json"))
	json.Unmarshal(data, &build)
	if !strings.Contains(strings.Join(build, " "), "-am clean package") {
		t.Fatalf("clean rebuild missing: %v", build)
	}
}

func isRunning(a *App, id string) bool { a.mu.Lock(); defer a.mu.Unlock(); return a.runs[id] != nil }

func TestFailedMavenBuildNeverStartsOldJar(t *testing.T) {
	p, _ := mavenFixture(t)
	if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
		t.Fatal(err)
	}
	p.Environment["RUNNER_TEST_BUILD"] = "fail"
	a := NewApp()
	a.projects[p.ID] = p
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	status := waitStatus(t, a, p.ID, "failed")
	if !strings.Contains(status.Error, "构建失败") {
		t.Fatalf("wrong error: %+v", status)
	}
	if exists(p.Environment["RUNNER_TEST_RUN_ARGS"]) || canConnect(p.Port) {
		t.Fatal("old artifact was launched after build failure")
	}
	found := false
	for _, line := range a.GetLogs(p.ID) {
		if strings.Contains(line.Text, "Current source does not compile") && line.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("final error output was lost: %+v", a.GetLogs(p.ID))
	}
}

func TestCancellingMavenBuildStopsChildrenAndPreventsRun(t *testing.T) {
	p, _ := mavenFixture(t)
	p.Environment["RUNNER_TEST_BUILD"] = "wait"
	a := NewApp()
	a.projects[p.ID] = p
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if isRunning(a, p.ID) {
			_ = a.StopProject(p.ID)
			waitStatus(t, a, p.ID, "stopped")
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for !canConnect(p.Port) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !canConnect(p.Port) {
		t.Fatalf("build child never started: %+v", a.GetLogs(p.ID))
	}
	if a.GetStatuses()[0].State != "building" {
		t.Fatal("build child port incorrectly marked application ready")
	}
	other := p
	other.ID = "other"
	other.Port = freePort(t)
	a.projects[other.ID] = other
	if err := a.StartProject(other.ID); err == nil || !strings.Contains(err.Error(), "同一工作树") {
		t.Fatalf("parallel build was not blocked: %v", err)
	}
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	if canConnect(p.Port) || exists(p.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("cancelled build left a child or launched application")
	}
}

func TestMavenModuleDirectoryFindsRootAndDetectsStartupModule(t *testing.T) {
	p, root := mavenFixture(t)
	p.Directory, p.Module = filepath.Join(root, "app"), ""
	plan, err := buildLaunchPlan(p, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Build.Directory != root {
		t.Fatalf("root=%q want=%q", plan.Build.Directory, root)
	}
	detection, err := NewApp().DetectProject(root)
	if err != nil || detection.Module != "app" {
		t.Fatalf("detection=%+v error=%v", detection, err)
	}
	p.Directory = root
	plan, err = buildLaunchPlan(p, false)
	if err != nil || plan.Build.Directory != root {
		t.Fatalf("automatic module selection failed: %+v %v", plan, err)
	}
}

func TestArtifactSelectionDoesNotFallBackToStaleJar(t *testing.T) {
	p, root := mavenFixture(t)
	module := filepath.Join(root, "app")
	if err := writeExecutableJar(filepath.Join(module, "target", "old-version.jar")); err != nil {
		t.Fatal(err)
	}
	if _, err := mavenArtifact(root, module); err == nil {
		t.Fatal("fell back to a stale artifact")
	}
	jar := p.Environment["RUNNER_TEST_JAR"]
	file, err := os.Create(jar)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("META-INF/MANIFEST.MF")
	entry.Write([]byte("Manifest-Version: 1.0\r\n"))
	archive.Close()
	file.Close()
	if _, err := mavenArtifact(root, module); err == nil || !strings.Contains(err.Error(), "不是可执行 JAR") {
		t.Fatalf("non-executable jar accepted: %v", err)
	}
}

func TestSeparateWorktreesCanRunIndependently(t *testing.T) {
	first, _ := mavenFixture(t)
	second, _ := mavenFixture(t)
	second.ID = "second"
	a := NewApp()
	a.projects[first.ID], a.projects[second.ID] = first, second
	for _, p := range []Project{first, second} {
		if err := a.StartProject(p.ID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if isRunning(a, p.ID) {
				_ = a.StopProject(p.ID)
				waitStatus(t, a, p.ID, "stopped")
			}
		})
	}
	waitStatus(t, a, first.ID, "running")
	waitStatus(t, a, second.ID, "running")
	if err := a.StopProject(first.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, first.ID, "stopped")
	if !canConnect(second.Port) {
		t.Fatal("stopping one worktree stopped another")
	}
}

func TestBatchCommandPreservesSpacesChineseAndMetacharacters(t *testing.T) {
	p, root := mavenFixture(t)
	tool := filepath.Join(root, "Maven 工具 (1)", "record.cmd")
	java := filepath.Join(p.JavaHome, "bin", "java.exe")
	if err := os.WriteFile(tool, []byte("@echo off\r\n\"%RUNNER_TEST_JAVA%\" record %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "record.json")
	args := []string{"value with spaces", "中文 & (test)", `C:\path with spaces\`, "", `-Dvalue=\"quoted space\"`}
	spec := commandSpec{Executable: tool, Directory: root, Env: append(os.Environ(), "RUNNER_TEST_HELPER=1", "RUNNER_TEST_RECORD="+record, "RUNNER_TEST_JAVA="+java), Args: args}
	cmd, err := managedCommand(spec)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("batch failed: %v %s", err, output)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	json.Unmarshal(data, &got)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("args=%q want=%q", got, args)
	}
}

func TestRealMavenBuildOnly(t *testing.T) {
	config := os.Getenv("RUNNER_MAVEN_VERIFY_CONFIG")
	if config == "" {
		t.Skip("set RUNNER_MAVEN_VERIFY_CONFIG to verify a configured project; builds only, never starts the service")
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.Kind != "spring-maven" {
			continue
		}
		// Exercise the normal long path, rather than relying on an 8.3 alias.
		if tool := os.Getenv("RUNNER_MAVEN_VERIFY_TOOL"); tool != "" {
			p.ToolPath = tool
		}
		plan, err := buildLaunchPlan(p, false)
		if err != nil {
			t.Fatal(err)
		}
		needed, reason := plan.Cache.needsBuild()
		t.Log(reason)
		if needed {
			if err := checkMavenArtifactUnlocked(plan.Cache.root, plan.Cache.module); err != nil {
				t.Fatal(err)
			}
			if err := plan.Cache.invalidate(); err != nil {
				t.Fatal(err)
			}
			cmd, err := managedCommand(*plan.Build)
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("reactor build failed: %v\n%s", err, output)
			}
			if err := plan.Cache.record(); err != nil {
				t.Fatal(err)
			}
		}
		spec, err := plan.Next()
		if err != nil {
			t.Fatal(err)
		}
		checkStarted := time.Now()
		unchanged, err := buildLaunchPlan(p, false)
		if err != nil {
			t.Fatal(err)
		}
		if needed, reason := unchanged.Cache.needsBuild(); needed {
			t.Fatalf("verified workspace not reusable: %s", reason)
		}
		t.Logf("Unchanged-input check completed in %s; next launch skips Maven", time.Since(checkStarted).Round(time.Millisecond))
		t.Logf("Reactor build succeeded; runtime executable=%s (not started)", spec.Executable)
		return
	}
	t.Fatal("configuration contains no Maven project")
}
