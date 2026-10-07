//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitFixtureCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
func gitFixtureFile(t *testing.T, root, path, text string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func gitFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	root := t.TempDir()
	gitFixtureCommand(t, root, "init", "-b", "main")
	gitFixtureCommand(t, root, "config", "user.email", "test@example.invalid")
	gitFixtureCommand(t, root, "config", "user.name", "Git view test")
	gitFixtureCommand(t, root, "config", "core.autocrlf", "false")
	gitFixtureCommand(t, root, "config", "core.hooksPath", filepath.Join(root, "no-hooks"))
	return root
}

func TestGitUncommittedViewsAreReadOnlyAndKeepStagesSeparate(t *testing.T) {
	root := gitFixture(t)
	for _, name := range []string{"both.txt", "old name.txt", "deleted.txt", "[x].txt", "x.txt"} {
		gitFixtureFile(t, root, name, "base\n")
	}
	gitFixtureFile(t, root, ".gitignore", "ignored.txt\n")
	gitFixtureCommand(t, root, "add", ".")
	gitFixtureCommand(t, root, "commit", "-m", "initial")
	gitFixtureFile(t, root, "both.txt", "base\nstaged\n")
	gitFixtureCommand(t, root, "add", "both.txt")
	gitFixtureFile(t, root, "both.txt", "base\nstaged\nworking\n")
	if err := os.Rename(filepath.Join(root, "old name.txt"), filepath.Join(root, "新名字.txt")); err != nil {
		t.Fatal(err)
	}
	gitFixtureCommand(t, root, "add", "--", "old name.txt", "新名字.txt")
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	gitFixtureFile(t, root, "未跟踪 文件.txt", "<img onerror=alert(1)>\n")
	gitFixtureFile(t, root, "ignored.txt", "ignore me")
	gitFixtureFile(t, root, "[x].txt", "literal file\n")
	gitFixtureFile(t, root, "x.txt", "other file\n")
	gitFixtureFile(t, root, "ui/.keep", "")
	index := filepath.Join(root, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	// Neither inherited repository overrides nor configured external tools may
	// redirect the read or execute code while viewing status and diff.
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "wrong-index"))
	helper := filepath.Join(root, "helper.cmd")
	marker := filepath.Join(root, "executed.txt")
	gitFixtureFile(t, root, "helper.cmd", "@echo off\r\necho unexpected > \""+marker+"\"\r\n")
	gitFixtureCommand(t, root, "config", "diff.external", helper)
	gitFixtureCommand(t, root, "config", "core.fsmonitor", helper)
	app := NewApp()
	changes, err := app.GetGitChanges(filepath.Join(root, "ui"))
	if err != nil {
		t.Fatal(err)
	}
	if changes.Root != root || changes.Branch != "main" || changes.Detached {
		t.Fatalf("wrong checkout: %+v", changes)
	}
	files := map[string]GitChange{}
	for _, file := range changes.Files {
		files[file.Path] = file
	}
	if !files["both.txt"].Staged || !files["both.txt"].Unstaged {
		t.Fatalf("stages lost: %+v", files)
	}
	if files["新名字.txt"].OldPath != "old name.txt" || !files["新名字.txt"].Staged {
		t.Fatalf("rename lost: %+v", files)
	}
	if !files["未跟踪 文件.txt"].Untracked || files["deleted.txt"].WorktreeStatus != "D" {
		t.Fatalf("states lost: %+v", files)
	}
	if _, found := files["ignored.txt"]; found {
		t.Fatal("ignored files included")
	}
	staged, err := app.GetGitFileDiff(root, "both.txt", "staged")
	if err != nil {
		t.Fatal(err)
	}
	working, err := app.GetGitFileDiff(root, "both.txt", "working")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.Text, "+staged") || strings.Contains(staged.Text, "+working") || !strings.Contains(working.Text, "+working") {
		t.Fatalf("wrong stage separation: %q / %q", staged.Text, working.Text)
	}
	literal, err := app.GetGitFileDiff(root, "[x].txt", "working")
	if err != nil || !strings.Contains(literal.Text, "+literal file") || strings.Contains(literal.Text, "other file") {
		t.Fatalf("literal path lost: %+v %v", literal, err)
	}
	preview, err := app.GetGitFileDiff(root, "未跟踪 文件.txt", "working")
	if err != nil || preview.Text != "<img onerror=alert(1)>\n" || !preview.Untracked {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err = app.GetGitFileDiff(root, "../outside.txt", "working"); err == nil {
		t.Fatal("escaped root")
	}
	if _, err = app.GetGitFileDiff(root, ".git/config", "working"); err == nil {
		t.Fatal("non-member preview allowed")
	}
	after, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only viewing changed the Git index")
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("external Git tool executed")
	}
}

func TestGitLinkedWorktreeDetachedAndUnbornBranch(t *testing.T) {
	root := gitFixture(t)
	app := NewApp()
	unborn, err := app.GetGitChanges(root)
	if err != nil || unborn.Branch != "main" || unborn.Files == nil || len(unborn.Files) != 0 {
		t.Fatalf("unborn: %+v %v", unborn, err)
	}
	gitFixtureFile(t, root, "file.txt", "base\n")
	gitFixtureCommand(t, root, "add", ".")
	gitFixtureCommand(t, root, "commit", "-m", "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	gitFixtureCommand(t, root, "worktree", "add", "-b", "feature", linked)
	gitFixtureFile(t, linked, "file.txt", "branch change\n")
	view, err := app.GetGitChanges(linked)
	if err != nil || view.Root != linked || view.Branch != "feature" || len(view.Files) != 1 {
		t.Fatalf("linked: %+v %v", view, err)
	}
	parent, err := app.GetGitChanges(root)
	if err != nil || len(parent.Files) != 0 {
		t.Fatalf("parent mixed: %+v %v", parent, err)
	}
	gitFixtureCommand(t, linked, "checkout", "--detach")
	view, err = app.GetGitChanges(linked)
	if err != nil || !view.Detached || view.Branch == "" {
		t.Fatalf("detached: %+v %v", view, err)
	}
	if _, err = app.GetGitChanges(t.TempDir()); err == nil {
		t.Fatal("non-repository accepted")
	}
}

func TestGitUntrackedBinaryAndBoundedPreview(t *testing.T) {
	root := gitFixture(t)
	gitFixtureFile(t, root, "binary.bin", "a\x00b")
	gitFixtureFile(t, root, "large.txt", strings.Repeat("x", 2*1024*1024))
	app := NewApp()
	binary, err := app.GetGitFileDiff(root, "binary.bin", "working")
	if err != nil || !binary.Binary || binary.Text != "" {
		t.Fatalf("binary: %+v %v", binary, err)
	}
	large, err := app.GetGitFileDiff(root, "large.txt", "working")
	if err != nil || !large.Truncated || len(large.Text) != 1024*1024 {
		t.Fatalf("unbounded preview: len=%d %v", len(large.Text), err)
	}
	files, err := parseGitChanges([]byte("D  same.txt\x00?? same.txt\x00UU 冲突.txt\x00"))
	if err != nil || len(files) != 2 || !files[0].Staged || !files[0].Untracked || !files[1].Conflict {
		t.Fatalf("status merge: %+v %v", files, err)
	}
}
