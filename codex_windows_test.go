//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type codexRecord struct {
	Args                              []string
	Directory, SnapshotPath, Snapshot string
}

func codexHelperProcess(args []string) int {
	if slices.Contains(args, "models") {
		if os.Getenv("RUNNER_TEST_CODEX_CATALOG_FAIL") == "1" {
			return 1
		}
		if os.Getenv("RUNNER_TEST_CODEX_CATALOG_WAIT") == "1" {
			for {
				time.Sleep(time.Second)
			}
		}
		// A different desktop cache must not leak into the CLI's catalog.
		fmt.Println(`{"models":[{"slug":"default-model","display_name":"默认模型","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]},{"slug":"hidden","visibility":"hide"}]}`)
		return 0
	}
	directory, _ := os.Getwd()
	record := codexRecord{Args: args, Directory: directory}
	if len(args) > 0 {
		_, tail, ok := strings.Cut(args[len(args)-1], "日志快照：")
		if ok {
			record.SnapshotPath, _, _ = strings.Cut(tail, "。沿最后")
		}
		data, _ := os.ReadFile(record.SnapshotPath)
		record.Snapshot = string(data)
	}
	data, _ := json.Marshal(record)
	_ = os.WriteFile(os.Getenv("RUNNER_TEST_CODEX_RECORD"), data, 0600)
	fmt.Println("CODEX_FIXTURE_READY")
	if os.Getenv("RUNNER_TEST_CODEX_EXIT") == "1" {
		return 17
	}
	for {
		time.Sleep(time.Second)
	}
}

func codexTestApp(t *testing.T) (*App, string) {
	t.Helper()
	a := terminalTestApp(t)
	root := t.TempDir()
	tool := filepath.Join(root, "CLI 工具 ' & demo")
	config := filepath.Join(root, "codex config")
	for _, dir := range []string{tool, config} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	executable, _ := os.Executable()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tool, "codex.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.toml"), []byte("model = \"default-model\"\nmodel_reasoning_effort = \"high\"\n[profiles.demo]\nmodel = \"wrong-profile\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache := `{"models":[{"slug":"desktop-only-model","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]}]}`
	if err := os.WriteFile(filepath.Join(config, "models_cache.json"), []byte(cache), 0600); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "record.json")
	p := a.projects["test"]
	for _, env := range []map[string]string{p.Environment, p.Frontend.Environment} {
		env["PATH"], env["CODEX_HOME"] = tool+";"+os.Getenv("PATH"), config
		env["RUNNER_TEST_HELPER"], env["RUNNER_TEST_CODEX"], env["RUNNER_TEST_CODEX_RECORD"] = "1", "1", record
	}
	a.projects["test"] = p
	a.history["test"] = []LogLine{{Text: "BACKEND_START"}, {Level: "error", Text: "Caused by: ClassNotFoundException: sample.Entity password=hidden-password"}}
	a.history["test:frontend"] = []LogLine{{Level: "error", Text: "FRONTEND_ONLY: Port 82 is already in use"}}
	exit := 1
	a.statuses["test"] = Status{ID: "test", State: "failed", ExitCode: &exit}
	return a, record
}

func readCodexRecord(t *testing.T, path string) codexRecord {
	t.Helper()
	var record codexRecord
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestCodexLowReadOnlyContextAndClosingCleanup(t *testing.T) {
	a, path := codexTestApp(t)
	options, err := a.GetCodexOptions("test")
	if err != nil || options.DefaultModel != "default-model" || len(options.Models) != 1 {
		t.Fatalf("options: %+v %v", options, err)
	}
	info, err := a.OpenCodexAnalysis("test", 160, 24, "", "")
	if err != nil {
		t.Fatal(err)
	}
	waitTerminalText(t, a, info.ID, "CODEX_FIXTURE_READY")
	record := readCodexRecord(t, path)
	args := strings.Join(record.Args, "|")
	for _, expected := range []string{"--sandbox|read-only", "--ask-for-approval|never", "--no-alt-screen", "--disable|hooks", "model_reasoning_effort=low"} {
		if !strings.Contains(strings.ReplaceAll(args, `"`, ""), expected) {
			t.Fatalf("missing %s in %s", expected, args)
		}
	}
	if !strings.Contains(args, "--model=default-model") || !strings.Contains(info.Title, "low") {
		t.Fatal("default must select a CLI-supported model and override reasoning to low")
	}
	if record.Directory != a.projects["test"].Directory || !strings.Contains(record.Snapshot, "ClassNotFoundException") || !strings.Contains(record.Snapshot, "退出码：1") || strings.Contains(record.Snapshot, "FRONTEND_ONLY") || strings.Contains(record.Snapshot, "hidden-password") {
		t.Fatalf("incorrect snapshot: %+v", record)
	}
	if _, err := os.Stat(record.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := a.CloseTerminal(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(record.SnapshotPath)); !os.IsNotExist(err) {
		t.Fatalf("snapshot was not cleaned: %v", err)
	}
}

func TestCodexFrontendModelAndNaturalExit(t *testing.T) {
	a, path := codexTestApp(t)
	a.projects["test"].Frontend.Environment["RUNNER_TEST_CODEX_EXIT"] = "1"
	info, err := a.OpenCodexAnalysis("test:frontend", 160, 24, "default-model", "medium")
	if err != nil {
		t.Fatal(err)
	}
	waitTerminalText(t, a, info.ID, "CODEX_FIXTURE_READY")
	s, _ := a.terminal(info.ID)
	select {
	case <-s.done:
	case <-time.After(15 * time.Second):
		t.Fatal("Codex parent shell did not exit")
	}
	record := readCodexRecord(t, path)
	if record.Directory != a.projects["test"].Frontend.Directory || !strings.Contains(record.Snapshot, "FRONTEND_ONLY") || strings.Contains(record.Snapshot, "BACKEND_START") || !strings.Contains(strings.Join(record.Args, "|"), "--model=default-model") {
		t.Fatalf("frontend context: %+v", record)
	}
	if state := s.snapshot().Info; state.State != "exited" || state.ExitCode == nil || *state.ExitCode != 17 {
		t.Fatalf("exit: %+v", state)
	}
	if _, err := os.Stat(record.SnapshotPath); !os.IsNotExist(err) {
		t.Fatal("natural exit retained snapshot")
	}
}

func TestCodexMissingLogsExecutableAndInvalidEffort(t *testing.T) {
	a, _ := codexTestApp(t)
	if _, err := a.OpenCodexAnalysis("missing", 80, 24, "", "low"); err == nil {
		t.Fatal("accepted unknown service")
	}
	delete(a.history, "test:frontend")
	if _, err := a.OpenCodexAnalysis("test:frontend", 80, 24, "", "low"); err == nil || !strings.Contains(err.Error(), "没有运行日志") {
		t.Fatalf("missing logs: %v", err)
	}
	if _, err := a.OpenCodexAnalysis("test", 80, 24, "default-model", "high"); err == nil {
		t.Fatal("accepted unsupported effort")
	}
	p := a.projects["test"]
	p.Environment["PATH"] = t.TempDir()
	a.projects["test"] = p
	if _, err := a.OpenCodexAnalysis("test", 80, 24, "", "low"); err == nil || !strings.Contains(err.Error(), "找不到 Codex") {
		t.Fatalf("missing executable: %v", err)
	}
	if len(a.GetTerminals()) != 0 {
		t.Fatal("failed launches left terminal tabs")
	}
}

func TestCodexUsesCLICatalogWhenConfiguredModelIsDesktopOnly(t *testing.T) {
	a, path := codexTestApp(t)
	p := a.projects["test"]
	config := filepath.Join(p.Environment["CODEX_HOME"], "config.toml")
	if err := os.WriteFile(config, []byte("model = \"desktop-only-model\"\nmodel_reasoning_effort = \"high\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	options, err := a.GetCodexOptions("test")
	if err != nil || options.DefaultModel != "default-model" || options.ConfiguredModel != "desktop-only-model" || len(options.Models) != 1 || options.Models[0].ID != "default-model" {
		t.Fatalf("desktop model was not replaced by CLI catalog selection: %+v %v", options, err)
	}
	info, err := a.OpenCodexAnalysis("test", 160, 30, "", "low")
	if err != nil {
		t.Fatal(err)
	}
	waitTerminalText(t, a, info.ID, "CODEX_FIXTURE_READY")
	if !slices.Contains(readCodexRecord(t, path).Args, "--model=default-model") {
		t.Fatal("CLI default was not passed explicitly to override the desktop config")
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), "desktop-only-model") {
		t.Fatal("analysis changed the user's global configuration")
	}
}

func TestCodexCatalogFailureKeepsManualModelsAndCustomProviders(t *testing.T) {
	a, path := codexTestApp(t)
	p := a.projects["test"]
	p.Environment["RUNNER_TEST_CODEX_CATALOG_FAIL"] = "1"
	a.projects["test"] = p
	if _, err := a.OpenCodexAnalysis("test", 80, 24, "", "low"); err == nil || !strings.Contains(err.Error(), "默认模型") {
		t.Fatalf("failed catalog query inherited an unchecked default: %v", err)
	}
	info, err := a.OpenCodexAnalysis("test", 160, 30, "manual-model", "low")
	if err != nil {
		t.Fatal(err)
	}
	waitTerminalText(t, a, info.ID, "CODEX_FIXTURE_READY")
	if !slices.Contains(readCodexRecord(t, path).Args, "--model=manual-model") {
		t.Fatal("manual model was lost")
	}
	config := filepath.Join(p.Environment["CODEX_HOME"], "config.toml")
	if err := os.WriteFile(config, []byte("model = \"custom-model\"\nmodel_provider = \"custom\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	options, err := a.GetCodexOptions("test")
	if err != nil || options.DefaultModel != "custom-model" {
		t.Fatalf("custom provider model was replaced: %+v %v", options, err)
	}
}

func TestCodexCatalogProbeTimeout(t *testing.T) {
	a, _ := codexTestApp(t)
	p := a.projects["test"]
	p.Environment["RUNNER_TEST_CODEX_CATALOG_WAIT"] = "1"
	env, _ := terminalEnvironment(p)
	executable, _ := commandOnPath("codex.exe", env)
	start := time.Now()
	if _, err := codexCatalog(executable, p.Directory, env, 200*time.Millisecond); err == nil {
		t.Fatal("hanging catalog query did not fail")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("catalog query exceeded its timeout")
	}
}

func TestCodexSnapshotBoundsRootCauseAndSecretRedaction(t *testing.T) {
	logs := make([]LogLine, 700)
	for i := range logs {
		logs[i].Text = fmt.Sprintf("line %d", i)
	}
	logs[699].Text = "Caused by: root password=pw123 token=tok123 Authorization: Bearer secret123 https://name:pw456@host"
	text := codexDiagnosis(Project{}, Status{}, logs)
	for _, secret := range []string{"pw123", "tok123", "secret123", "pw456"} {
		if strings.Contains(text, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !strings.Contains(text, "最近 400 行") || strings.Contains(text, "line 299\n") || !strings.Contains(text, "Caused by: root") {
		t.Fatal("snapshot did not retain latest root cause")
	}
	logs[699].Text = strings.Repeat("大", 40000) + "ROOT_AT_END"
	text = codexDiagnosis(Project{}, Status{}, logs)
	if !strings.Contains(text, "ROOT_AT_END") || !strings.Contains(text, "本行过长") || len(text) > 270000 {
		t.Fatal("snapshot bounds failed")
	}
}

func TestInstalledCodexHelpRunsInsidePseudoConsole(t *testing.T) {
	a := terminalTestApp(t)
	p := a.projects["test"]
	env, _ := terminalEnvironment(p)
	options := codexOptions(env, p.Directory)
	if options.Executable == "" {
		t.Skip("Codex CLI not installed")
	}
	a.groupMu.Lock()
	info, err := a.newTerminalLocked("test", 160, 30, "Codex help", "& "+powershellLiteral(options.Executable)+" --help; exit $LASTEXITCODE", nil)
	a.groupMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	waitTerminalText(t, a, info.ID, "--no-alt-screen")
}

func TestInstalledCodexHooksDisabledInsidePseudoConsole(t *testing.T) {
	a := terminalTestApp(t)
	env, _ := terminalEnvironment(a.projects["test"])
	options := codexOptions(env, a.projects["test"].Directory)
	if options.Executable == "" {
		t.Skip("Codex CLI not installed")
	}
	a.groupMu.Lock()
	info, err := a.newTerminalLocked("test", 160, 30, "Codex features", "& "+powershellLiteral(options.Executable)+" --disable hooks features list; exit $LASTEXITCODE", nil)
	a.groupMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := a.terminal(info.ID)
	select {
	case <-s.done:
	case <-time.After(15 * time.Second):
		t.Fatal("Codex feature inspection did not finish")
	}
	text := waitTerminalText(t, a, info.ID, "hooks")
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "hooks" {
			if fields[len(fields)-1] != "false" {
				t.Fatalf("hooks remain enabled: %s", line)
			}
			return
		}
	}
	t.Fatalf("CLI did not report the hooks feature: %s", text)
}
