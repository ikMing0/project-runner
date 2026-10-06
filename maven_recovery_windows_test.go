//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type recoveryStage struct {
	Kind string
	Args []string
}

func recordRecoveryStage(kind string, args []string) {
	path := os.Getenv("RUNNER_TEST_RECOVERY_STAGES")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	data, _ := json.Marshal(recoveryStage{kind, args})
	_, _ = file.Write(append(data, '\n'))
}

func recoveryFixture(t *testing.T, p Project, seed bool) Project {
	t.Helper()
	p.Environment["RUNNER_TEST_RECOVERY_STAGES"] = filepath.Join(p.Directory, "recovery-stages.jsonl")
	if seed {
		c := fixtureCache(t, p)
		c.needsBuild()
		if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
			t.Fatal(err)
		}
		if err := c.record(); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func recoveryStages(t *testing.T, p Project) []recoveryStage {
	t.Helper()
	data, err := os.ReadFile(p.Environment["RUNNER_TEST_RECOVERY_STAGES"])
	if err != nil {
		t.Fatal(err)
	}
	var stages []recoveryStage
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var stage recoveryStage
		if err := json.Unmarshal([]byte(line), &stage); err != nil {
			t.Fatal(err)
		}
		stages = append(stages, stage)
	}
	return stages
}

func staleRecoveryResource(t *testing.T, p Project) {
	t.Helper()
	path := filepath.Join(p.Directory, "app", "target", "classes", "mapper", "StaleMapper.xml")
	writeCacheInput(t, path, "stale mapper referencing a removed class")
	p.Environment["RUNNER_TEST_STALE_RESOURCE"] = path
}

func TestFirstAndUntrustedMavenBuildCleansStaleResources(t *testing.T) {
	for _, mode := range []string{"first", "old-version", "corrupt-record"} {
		t.Run(mode, func(t *testing.T) {
			p, _ := mavenFixture(t)
			p = recoveryFixture(t, p, mode != "first")
			if mode != "first" {
				c := fixtureCache(t, p)
				data, _ := os.ReadFile(c.path)
				if mode == "old-version" {
					var record mavenBuildRecord
					_ = json.Unmarshal(data, &record)
					record.Version = 1
					data, _ = json.Marshal(record)
				} else {
					data = []byte("invalid record")
				}
				writeCacheInput(t, c.path, string(data))
			}
			staleRecoveryResource(t, p)
			a := NewApp()
			a.projects[p.ID] = p
			t.Cleanup(func() { a.shutdown(nil) })
			if err := a.StartProject(p.ID); err != nil {
				t.Fatal(err)
			}
			status := waitStatus(t, a, p.ID, "running")
			stages := recoveryStages(t, p)
			if status.Attempt != 1 || status.Recovery != "" || len(stages) != 2 || stages[0].Kind != "build" || !mavenCommandIsClean(&commandSpec{Args: stages[0].Args}) || exists(p.Environment["RUNNER_TEST_STALE_RESOURCE"]) {
				t.Fatalf("untrusted output was not cleaned before launch: %+v %+v", status, stages)
			}
		})
	}
}

func TestMavenArtifactRecoveryKeepsFrontendAndReusesRepairedJar(t *testing.T) {
	p, a := pairedFixture(t)
	p = recoveryFixture(t, p, true)
	staleRecoveryResource(t, p)
	a.projects[p.ID] = p
	if err := a.StartService(frontendID(p.ID)); err != nil {
		t.Fatal(err)
	}
	frontend := waitStatus(t, a, frontendID(p.ID), "running")
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	status := waitStatus(t, a, p.ID, "running")
	stages := recoveryStages(t, p)
	if status.Recovery != "recovered" || status.Attempt != 2 || status.StartupDurationMs == nil || len(stages) != 3 || stages[0].Kind != "start" || stages[1].Kind != "build" || !mavenCommandIsClean(&commandSpec{Args: stages[1].Args}) || stages[2].Kind != "start" {
		t.Fatalf("expected one clean recovery: %+v %+v", status, stages)
	}
	if got := waitStatus(t, a, frontendID(p.ID), "running"); got.PID != frontend.PID {
		t.Fatal("backend recovery restarted the frontend")
	}
	var firstFailure, recovered bool
	for _, line := range a.GetLogs(p.ID) {
		if line.Attempt == 1 && strings.Contains(line.Text, "ClassNotFoundException") {
			firstFailure = true
		}
		if line.Attempt == 2 && strings.Contains(line.Text, "已自动恢复") {
			recovered = true
		}
	}
	if !firstFailure || !recovered {
		t.Fatalf("attempt logs were lost: %+v", a.GetLogs(p.ID))
	}
	if err := a.StopService(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	if err := a.StartService(p.ID); err != nil {
		t.Fatal(err)
	}
	status = waitStatus(t, a, p.ID, "running")
	if status.Attempt != 1 || status.Recovery != "" || len(recoveryStages(t, p)) != 4 {
		t.Fatal("repaired artifact was not reused on the next start")
	}
}

func TestIncrementalMavenBuildCanRecoverOnce(t *testing.T) {
	p, root := mavenFixture(t)
	p = recoveryFixture(t, p, true)
	writeCacheInput(t, filepath.Join(root, "common", "src", "main", "java", "Changed.java"), "class Changed {}")
	staleRecoveryResource(t, p)
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	status := waitStatus(t, a, p.ID, "running")
	stages := recoveryStages(t, p)
	if status.Recovery != "recovered" || len(stages) != 4 || mavenCommandIsClean(&commandSpec{Args: stages[0].Args}) || !mavenCommandIsClean(&commandSpec{Args: stages[2].Args}) {
		t.Fatalf("incremental failure did not recover cleanly: %+v %+v", status, stages)
	}
}

func TestOrdinaryMavenStartupFailuresDoNotRebuildOrInvalidate(t *testing.T) {
	for _, failure := range []string{"database", "initialization"} {
		t.Run(failure, func(t *testing.T) {
			p, _ := mavenFixture(t)
			p = recoveryFixture(t, p, true)
			p.Environment["RUNNER_TEST_APP_ERROR"] = failure
			a := NewApp()
			a.projects[p.ID] = p
			t.Cleanup(func() { a.shutdown(nil) })
			if err := a.StartProject(p.ID); err != nil {
				t.Fatal(err)
			}
			status := waitStatus(t, a, p.ID, "failed")
			if status.Attempt != 1 || status.Recovery != "" || len(recoveryStages(t, p)) != 1 || !exists(fixtureCache(t, p).path) {
				t.Fatalf("ordinary runtime failure invalidated a build: %+v", status)
			}
		})
	}
}

func TestMavenRecoveryStopsAfterCleanStartupOrBuildFailure(t *testing.T) {
	for _, mode := range []string{"second-startup", "first-clean-startup", "recovery-build"} {
		t.Run(mode, func(t *testing.T) {
			p, _ := mavenFixture(t)
			p = recoveryFixture(t, p, mode != "first-clean-startup")
			p.Environment["RUNNER_TEST_APP_ERROR"] = "mapper"
			if mode == "recovery-build" {
				p.Environment["RUNNER_TEST_BUILD"] = "fail"
			}
			a := NewApp()
			a.projects[p.ID] = p
			t.Cleanup(func() { a.shutdown(nil) })
			if err := a.StartProject(p.ID); err != nil {
				t.Fatal(err)
			}
			status := waitStatus(t, a, p.ID, "failed")
			stages := recoveryStages(t, p)
			want := 3
			if mode != "second-startup" {
				want = 2
			}
			if len(stages) != want || exists(fixtureCache(t, p).path) {
				t.Fatalf("bad artifact retained or recovery looped: %+v %+v", status, stages)
			}
			if mode != "first-clean-startup" && (status.Recovery != "failed" || status.Attempt != 2) {
				t.Fatalf("recovery failure state: %+v", status)
			}
			if mode == "recovery-build" && !strings.Contains(status.Error, "构建失败") {
				t.Fatal("failed recovery build launched an old JAR")
			}
		})
	}
}

func TestStoppingMavenRecoveryStopsChildrenAndKeepsWorktreeLocked(t *testing.T) {
	p, _ := mavenFixture(t)
	p = recoveryFixture(t, p, true)
	staleRecoveryResource(t, p)
	p.Environment["RUNNER_TEST_BUILD"] = "wait"
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for !canConnect(p.Port) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !canConnect(p.Port) {
		t.Fatalf("recovery build child did not start: %+v", a.GetLogs(p.ID))
	}
	status := waitStatus(t, a, p.ID, "building")
	if status.Recovery != "building" {
		t.Fatalf("old readiness monitor marked recovery as ready: %+v", status)
	}
	other := p
	other.ID = "same-worktree"
	other.Port = freePort(t)
	a.mu.Lock()
	a.projects[other.ID] = other
	a.mu.Unlock()
	if err := a.StartProject(other.ID); err == nil || !strings.Contains(err.Error(), "同一工作树") {
		t.Fatalf("recovery released the worktree lock: %v", err)
	}
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	status = waitStatus(t, a, p.ID, "stopped")
	if status.Recovery != "cancelled" || canConnect(p.Port) || exists(fixtureCache(t, p).path) || len(recoveryStages(t, p)) != 2 {
		t.Fatalf("cancel did not stop recovery: %+v", status)
	}
}

func TestStaleReadinessCannotMarkAnotherAttemptRunning(t *testing.T) {
	a := NewApp()
	r := &run{started: time.Now()}
	old := r.stage.Add(1)
	current := r.stage.Add(1)
	a.runs["test"] = r
	a.statuses["test"] = Status{ID: "test", State: "starting", Attempt: 2, Recovery: "starting"}
	a.markRunningStage("test", r, old)
	if a.GetStatuses()[0].State != "starting" {
		t.Fatal("stale attempt marked the retry running")
	}
	a.markRunningStage("test", r, current)
	if a.GetStatuses()[0].Recovery != "recovered" {
		t.Fatal("current attempt did not report recovery")
	}
}

func TestMavenArtifactFailureRules(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{"mapper-class", []string{"Error parsing Mapper XML", "Caused by: java.lang.ClassNotFoundException: Cannot find class: sample.Entity"}, true},
		{"mapper-bytecode", []string{"Could not resolve type alias 'sample.Entity'", "java.lang.NoClassDefFoundError: sample/Entity"}, true},
		{"zip-corruption", []string{"java.util.zip.ZipException: invalid LOC header (bad signature)"}, true},
		{"database", []string{"Failed to instantiate SqlSessionFactory", "java.sql.SQLException: Connection refused"}, false},
		{"jdbc-driver", []string{"Failed to instantiate SqlSessionFactory", "java.lang.ClassNotFoundException: com.mysql.cj.jdbc.Driver"}, false},
		{"xml-syntax", []string{"Error parsing Mapper XML", "org.xml.sax.SAXParseException: invalid XML"}, false},
		{"initializer", []string{"Error parsing Mapper XML", "java.lang.NoClassDefFoundError: Could not initialize class sample.Entity"}, false},
		{"redis", []string{"RedisConnectionFailureException: Unable to connect"}, false},
		{"port", []string{"BindException: Address already in use"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &mavenArtifactFailure{}
			for _, line := range tc.lines {
				d.observe(line)
			}
			if (d.reason() != "") != tc.want {
				t.Fatalf("classification: %q", d.reason())
			}
		})
	}
	original := commandSpec{Args: []string{"-pl", "clean", "-am", "package", "-Dmaven.test.skip=true"}}
	prepared := mavenCleanCommand(original)
	if !mavenCommandIsClean(&prepared) || slices.Contains(original.Args[3:], "clean") || !slices.Equal(prepared.Args, mavenCleanCommand(prepared).Args) {
		t.Fatal("clean goal duplicated or modified the original command")
	}
}
