//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHistoryPersistsBoundedRedactedRecordsAcrossReopen(t *testing.T) {
	a := NewApp()
	a.configPath = filepath.Join(t.TempDir(), "projects.json")
	p := Project{Name: "fixture", Environment: map[string]string{"PRIVATE_TOKEN": "super-private-value"}}
	a.history["one"] = []LogLine{
		{Text: `{"password":"json-secret"} Bearer bearer-secret https://user:url-secret@localhost/path`},
		{Text: "super-private-value sk-123456789012345678901234"},
		{Text: "-----BEGIN PRIVATE KEY-----"},
		{Text: "some-private-key-body"},
		{Text: "-----END PRIVATE KEY-----"},
	}
	for i := range 12 {
		a.mu.Lock()
		a.archiveRunLocked("one", p, Status{State: "failed", StartedAt: int64(i + 1), Error: "token=exit-secret"})
		a.mu.Unlock()
	}
	reopened := NewApp()
	reopened.configPath = a.configPath
	records, err := reopened.ListRunHistory("one")
	if err != nil || len(records) != 10 || records[0].StartedAt != 12 || records[0].Logs != nil {
		t.Fatalf("metadata: %+v %v", records, err)
	}
	text, err := reopened.RunHistoryText(records[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(filepath.Dir(a.configPath), "run-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"json-secret", "bearer-secret", "url-secret", "super-private-value", "sk-123456789012345678901234", "some-private-key-body", "exit-secret"} {
		if strings.Contains(text, secret) || strings.Contains(string(disk), secret) {
			t.Fatalf("history disclosed %s", secret)
		}
	}
	if _, err := reopened.ReadRunHistory("../../projects.json"); err == nil {
		t.Fatal("history accepted non-record identifier")
	}
	oversized := make([]LogLine, 1500)
	for i := range oversized {
		oversized[i].Text = strings.Repeat("<secret-like-text>", 500)
	}
	logs := archiveLogs(oversized, p)
	encoded, _ := json.MarshalIndent(logs, "", "  ")
	if len(logs) > 1000 || len(encoded) > 256*1024 {
		t.Fatalf("history limits exceeded: lines=%d bytes=%d", len(logs), len(encoded))
	}
}

func TestHistoryKeepsGlobalLimitAcrossServices(t *testing.T) {
	a := NewApp()
	a.configPath = filepath.Join(t.TempDir(), "projects.json")
	for i := range 60 {
		id := string(rune('a' + i%6))
		a.mu.Lock()
		a.archiveRunLocked(id, Project{Name: id}, Status{StartedAt: int64(i)})
		a.mu.Unlock()
	}
	records, err := a.archiveRecords()
	if err != nil || len(records) != 50 {
		t.Fatalf("global archive bound %d %v", len(records), err)
	}
}

func TestTemplateLocalReuseAndShareAllowlist(t *testing.T) {
	p, a := pairedFixture(t)
	p.Health = &HealthConfig{URL: "http://localhost:8080/private-health?token=health-secret"}
	p.Environment["AUTH_TOKEN"] = "env-secret"
	p.JVMArgs = "-Dpassword=jvm-secret"
	p.AppArgs = "--secret=arg-secret"
	p.Frontend.Environment["PLAIN_NAME"] = "frontend-secret"
	p.Frontend.AppArgs = "--password=frontend-arg-secret"
	saved, err := a.SaveTemplate(p, "paired template")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, dir := range []string{"app", "ruoyi-ui"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "app", "pom.xml"), []byte("<project/>"), 0600); err != nil {
		t.Fatal(err)
	}
	applied, err := a.ApplyTemplate(saved, root)
	if err != nil {
		t.Fatal(err)
	}
	if applied.ID != "" || applied.Directory != root || applied.Frontend.Directory != filepath.Join(root, "ruoyi-ui") ||
		applied.Environment["AUTH_TOKEN"] != "env-secret" || applied.JavaHome != p.JavaHome {
		t.Fatalf("local template lost relative paths/local values: %+v", applied)
	}
	if len(a.ListProjects()) != 1 || len(a.runs) != 0 {
		t.Fatal("template application automatically saved or started project")
	}
	bundle := templateBundle(saved)
	data, _ := json.Marshal(bundle)
	for _, value := range []string{p.Directory, p.JavaHome, p.ConfigFile, "env-secret", "jvm-secret", "arg-secret", "frontend-secret", "health-secret"} {
		if strings.Contains(string(data), value) {
			t.Fatalf("share template leaked %s", value)
		}
	}
	shared := bundle.Templates[0]
	if len(shared.Project.Environment) != 0 || shared.Project.Health != nil || shared.Project.ToolPath != "" || shared.Project.Frontend.NodeHome != "" ||
		shared.FrontendRelative == "" {
		t.Fatal("share allowlist incomplete")
	}
	list, err := a.ListTemplates()
	if err != nil || len(list) != 1 || list[0].ID != saved.ID {
		t.Fatalf("template persistence %v", err)
	}
	if err := a.DeleteTemplate(saved.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = a.ListTemplates()
	if len(list) != 0 {
		t.Fatal("deleted template remained")
	}
}

func TestImportedTemplateRejectsTraversalAndDropsExecutableFields(t *testing.T) {
	template := ProjectTemplate{Name: "imported", Project: Project{Kind: "node", Directory: "C:/private", ToolPath: "C:/malicious.cmd",
		Environment: map[string]string{"TOKEN": "secret"}, AppArgs: "--secret=hidden"}}
	validated, err := validateImportedTemplate(template)
	if err != nil || validated.Project.ToolPath != "" || validated.Project.Directory != "" || len(validated.Project.Environment) != 0 {
		t.Fatalf("unsafe import: %+v %v", validated, err)
	}
	for _, path := range []string{"../outside", `..\outside`, "C:/outside", `\\host\share\outside`} {
		copy := template
		copy.FrontendRelative = path
		if _, err := validateImportedTemplate(copy); err == nil {
			t.Fatalf("accepted traversal %s", path)
		}
	}
	file := filepath.Join(t.TempDir(), "template.json")
	if err := privateJSON(file, templateBundle(template)); err != nil {
		t.Fatal(err)
	}
	templates, err := readTemplateBundle(file)
	if err != nil || len(templates) != 1 {
		t.Fatalf("bundle read %v", err)
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("x", 1024*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTemplateBundle(file); err == nil {
		t.Fatal("oversized import accepted")
	}
}
