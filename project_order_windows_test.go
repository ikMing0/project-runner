//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func orderFixture(t *testing.T) (*App, []Project) {
	t.Helper()
	root := t.TempDir()
	a := NewApp()
	a.configPath = filepath.Join(root, "projects.json")
	var projects []Project
	for _, id := range []string{"c", "b", "a"} {
		p, err := a.SaveProject(Project{ID: id, Name: id, Directory: root, Kind: "spring-maven", Port: 8080})
		if err != nil {
			t.Fatal(err)
		}
		projects = append(projects, p)
	}
	return a, projects
}

func assertProjectOrder(t *testing.T, a *App, expected ...string) {
	t.Helper()
	ids := make([]string, 0)
	for _, p := range a.ListProjects() {
		ids = append(ids, p.ID)
	}
	if !reflect.DeepEqual(ids, expected) {
		t.Fatalf("order = %v, want %v", ids, expected)
	}
}

func TestProjectOrderPersistsAcrossReopenRenameCreateAndDelete(t *testing.T) {
	a, projects := orderFixture(t)
	assertProjectOrder(t, a, "c", "b", "a")
	if err := a.ReorderProjects([]string{"a", "c", "b"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_RUNNER_CONFIG", a.configPath)
	reopened := NewApp()
	reopened.startup(nil)
	assertProjectOrder(t, reopened, "a", "c", "b")
	renamed := projects[0]
	renamed.Name = "00 renamed"
	if _, err := reopened.SaveProject(renamed); err != nil {
		t.Fatal(err)
	}
	copy := projects[0]
	copy.ID = ""
	copy.Name = "00 copied"
	saved, err := reopened.SaveProject(copy)
	if err != nil {
		t.Fatal(err)
	}
	assertProjectOrder(t, reopened, "a", "c", "b", saved.ID)
	if err := reopened.DeleteProject("c"); err != nil {
		t.Fatal(err)
	}
	assertProjectOrder(t, reopened, "a", "b", saved.ID)
	again := NewApp()
	again.startup(nil)
	assertProjectOrder(t, again, "a", "b", saved.ID)
}

func TestProjectOrderLoadsExistingArrayAndIgnoresDuplicateIDs(t *testing.T) {
	a, projects := orderFixture(t)
	legacy := []Project{projects[1], projects[0], projects[1], {Name: "invalid"}, projects[2]}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_RUNNER_CONFIG", a.configPath)
	reopened := NewApp()
	reopened.startup(nil)
	assertProjectOrder(t, reopened, "b", "c", "a")
}

func TestProjectOrderRejectsIncompleteDuplicateAndUnknownIDsWithoutChangingConfig(t *testing.T) {
	a, _ := orderFixture(t)
	before, err := os.ReadFile(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"a", "b"}, {"a", "b", "b"}, {"a", "b", "unknown"}, {"a", "b", "c", "extra"}} {
		if err := a.ReorderProjects(ids); err == nil {
			t.Fatalf("accepted invalid order %v", ids)
		}
		assertProjectOrder(t, a, "c", "b", "a")
		after, err := os.ReadFile(a.configPath)
		if err != nil || string(after) != string(before) {
			t.Fatal("invalid reorder changed saved config")
		}
	}
}

func TestProjectOrderSaveFailureRollsBackOrderCreateAndDelete(t *testing.T) {
	a, projects := orderFixture(t)
	if err := a.ReorderProjects([]string{"a", "c", "b"}); err != nil {
		t.Fatal(err)
	}
	// A regular file used as the parent directory reliably makes persistence fail.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	a.configPath = filepath.Join(blocker, "projects.json")
	if err := a.ReorderProjects([]string{"b", "c", "a"}); err == nil {
		t.Fatal("reorder unexpectedly saved")
	}
	assertProjectOrder(t, a, "a", "c", "b")
	copy := projects[0]
	copy.ID = "new"
	if _, err := a.SaveProject(copy); err == nil {
		t.Fatal("create unexpectedly saved")
	}
	assertProjectOrder(t, a, "a", "c", "b")
	if err := a.DeleteProject("c"); err == nil {
		t.Fatal("delete unexpectedly saved")
	}
	assertProjectOrder(t, a, "a", "c", "b")
}

func TestProjectOrderCanMoveRunningGroupWithoutChangingItsProcesses(t *testing.T) {
	a, _ := orderFixture(t)
	backend, frontend := &run{}, &run{}
	a.runs["b"] = backend
	a.runs[frontendID("b")] = frontend
	a.statuses["b"] = Status{ID: "b", State: "running", PID: 123}
	a.statuses[frontendID("b")] = Status{ID: frontendID("b"), State: "running", PID: 456}
	if err := a.ReorderProjects([]string{"b", "c", "a"}); err != nil {
		t.Fatal(err)
	}
	assertProjectOrder(t, a, "b", "c", "a")
	if a.runs["b"] != backend || a.runs[frontendID("b")] != frontend || a.statuses["b"].PID != 123 || a.statuses[frontendID("b")].PID != 456 {
		t.Fatal("reorder changed running services")
	}
}
