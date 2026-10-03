package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeNameDefaultsFromRootForNestedProject(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "feature-payment")
	project := filepath.Join(worktree, "web")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	// A linked Git worktree identifies its root with a .git file.
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ignored-for-name-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	detection, err := a.DetectProject(project)
	if err != nil {
		t.Fatal(err)
	}
	if detection.Name != "feature-payment" || detection.Kind != "node" {
		t.Fatalf("unexpected detection: %+v", detection)
	}

	t.Setenv("PROJECT_RUNNER_CONFIG", filepath.Join(t.TempDir(), "projects.json"))
	a.startup(nil)
	saved, err := a.SaveProject(Project{Directory: project, Kind: "node", Port: 5173, Script: "dev", PortMode: "vite"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "feature-payment" {
		t.Fatalf("default name = %q", saved.Name)
	}
	saved.Name = "我自己的名称"
	updated, err := a.SaveProject(saved)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != saved.Name {
		t.Fatalf("manual name changed to %q", updated.Name)
	}
}

func TestNameFallsBackToSelectedDirectoryWithoutGit(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "plain-project")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	detection, err := NewApp().DetectProject(directory)
	if err != nil {
		t.Fatal(err)
	}
	if detection.Name != "plain-project" || detection.Kind != "" {
		t.Fatalf("unexpected detection: %+v", detection)
	}
}
