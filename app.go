package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Project represents one runnable worktree or module.
type Project struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Directory      string            `json:"directory"`
	Kind           string            `json:"kind"` // spring-maven, spring-gradle, node
	Port           int               `json:"port"`
	ConfigFile     string            `json:"configFile"`
	ConfigProperty string            `json:"configProperty"`
	JavaHome       string            `json:"javaHome"`
	ToolPath       string            `json:"toolPath"`
	Module         string            `json:"module"`
	PackageManager string            `json:"packageManager"`
	Script         string            `json:"script"`
	PortMode       string            `json:"portMode"` // vite, vue-cli, env, none
	JVMArgs        string            `json:"jvmArgs"`
	AppArgs        string            `json:"appArgs"`
	Environment    map[string]string `json:"environment"`
	NodeHome       string            `json:"nodeHome"`
	Frontend       *FrontendConfig   `json:"frontend,omitempty"`
}

type Detection struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	PackageManager string   `json:"packageManager"`
	PortMode       string   `json:"portMode"`
	Scripts        []string `json:"scripts"`
	Module         string   `json:"module"`
	Modules        []string `json:"modules"`
	Script         string   `json:"script"`
}

type LogLine struct {
	Time   string `json:"time"`
	Source string `json:"source"`
	Level  string `json:"level"`
	Text   string `json:"text"`
}

type Status struct {
	ID                string `json:"id"`
	State             string `json:"state"`
	PID               int    `json:"pid"`
	ExitCode          *int   `json:"exitCode"`
	Error             string `json:"error"`
	StartedAt         int64  `json:"startedAt,omitempty"`
	StartupDurationMs *int64 `json:"startupDurationMs,omitempty"`
}

type App struct {
	ctx            context.Context
	mu             sync.Mutex
	projects       map[string]Project
	runs           map[string]*run
	history        map[string][]LogLine
	statuses       map[string]Status
	configPath     string
	groupMu        sync.Mutex
	closing        bool // guarded by groupMu
	terminalMu     sync.Mutex
	terminals      map[string]*terminalSession
	terminalNumber int
}

func NewApp() *App {
	return &App{projects: map[string]Project{}, runs: map[string]*run{}, history: map[string][]LogLine{}, statuses: map[string]Status{}, terminals: map[string]*terminalSession{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	path, err := configFilePath()
	if err != nil {
		return
	}
	a.configPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var projects []Project
	if json.Unmarshal(data, &projects) == nil {
		for _, p := range projects {
			if p.ID != "" {
				a.projects[p.ID] = p
			}
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	a.closing = true
	a.mu.Lock()
	runs := make([]*run, 0, len(a.runs))
	for _, r := range a.runs {
		runs = append(runs, r)
	}
	a.mu.Unlock()
	for _, r := range runs {
		r.stop()
	}
	a.closeProjectTerminals("")
	for _, r := range runs {
		<-r.done
	}
}

func configFilePath() (string, error) {
	if path := os.Getenv("PROJECT_RUNNER_CONFIG"); path != "" {
		return filepath.Abs(path)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ProjectRunner", "projects.json"), nil
}

func (a *App) saveLocked() error {
	if a.configPath == "" {
		return errors.New("无法确定配置目录")
	}
	projects := make([]Project, 0, len(a.projects))
	for _, p := range a.projects {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.configPath), 0700); err != nil {
		return err
	}
	tmp := a.configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.configPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (a *App) ListProjects() []Project {
	a.mu.Lock()
	defer a.mu.Unlock()
	projects := make([]Project, 0, len(a.projects))
	for _, p := range a.projects {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects
}

func (a *App) SaveProject(p Project) (Project, error) {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	if strings.TrimSpace(p.Directory) == "" {
		return Project{}, errors.New("请选择项目目录")
	}
	abs, err := filepath.Abs(p.Directory)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return Project{}, errors.New("项目目录不存在")
	}
	p.Directory = filepath.Clean(abs)
	if p.Port < 1 || p.Port > 65535 {
		return Project{}, errors.New("端口必须在 1–65535 之间")
	}
	if p.Kind != "spring-maven" && p.Kind != "spring-gradle" && p.Kind != "node" {
		return Project{}, errors.New("不支持的启动类型")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		p.Name = suggestedProjectName(p.Directory)
	}
	if p.ConfigProperty == "" {
		p.ConfigProperty = "application.config.path"
	}
	if p.Kind == "node" && p.Script == "" {
		return Project{}, errors.New("请选择 package.json 脚本")
	}
	if p.Kind == "node" && p.Frontend != nil {
		return Project{}, errors.New("配套前端只能关联到后端项目")
	}
	if p.NodeHome != "" {
		path, err := filepath.Abs(p.NodeHome)
		if err != nil || !exists(filepath.Join(path, "node.exe")) {
			return Project{}, errors.New("Node.js 目录无效，需要包含 node.exe")
		}
		p.NodeHome = filepath.Clean(path)
	}
	if p.Frontend != nil {
		frontend, err := validateFrontend(*p.Frontend, p.Port)
		if err != nil {
			return Project{}, err
		}
		p.Frontend = &frontend
	}
	if p.ConfigFile != "" {
		path, err := filepath.Abs(p.ConfigFile)
		if err != nil {
			return Project{}, err
		}
		if _, err := os.Stat(path); err != nil {
			return Project{}, fmt.Errorf("配置文件不存在: %w", err)
		}
		p.ConfigFile = filepath.Clean(path)
	}
	if p.ToolPath != "" {
		path, err := filepath.Abs(p.ToolPath)
		if err != nil {
			return Project{}, err
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return Project{}, errors.New("构建工具文件不存在")
		}
		p.ToolPath = filepath.Clean(path)
	}
	if p.ID == "" {
		bytes := make([]byte, 8)
		if _, err := rand.Read(bytes); err != nil {
			return Project{}, err
		}
		p.ID = hex.EncodeToString(bytes)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runs[p.ID] != nil || a.runs[frontendID(p.ID)] != nil {
		return Project{}, errors.New("请先停止项目，再修改配置")
	}
	previous, existed := a.projects[p.ID]
	a.projects[p.ID] = p
	if err := a.saveLocked(); err != nil {
		if existed {
			a.projects[p.ID] = previous
		} else {
			delete(a.projects, p.ID)
		}
		return Project{}, err
	}
	return p, nil
}

func (a *App) DeleteProject(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runs[id] != nil || a.runs[frontendID(id)] != nil {
		return errors.New("请先停止项目")
	}
	p, found := a.projects[id]
	if !found {
		return errors.New("项目不存在")
	}
	delete(a.projects, id)
	if err := a.saveLocked(); err != nil {
		a.projects[id] = p
		return err
	}
	delete(a.history, id)
	delete(a.statuses, id)
	delete(a.history, frontendID(id))
	delete(a.statuses, frontendID(id))
	a.closeProjectTerminals(id)
	return nil
}

func (a *App) GetStatuses() []Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	statuses := make([]Status, 0, len(a.statuses))
	for _, s := range a.statuses {
		statuses = append(statuses, s)
	}
	return statuses
}

func (a *App) GetLogs(id string) []LogLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]LogLine{}, a.history[id]...)
}

func (a *App) PickDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择项目目录"})
}

func (a *App) PickConfigFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择本地配置文件"})
}

func (a *App) PickToolFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择 Maven 或 Gradle 启动文件"})
}

func (a *App) DetectProject(directory string) (Detection, error) {
	if directory == "" {
		return Detection{}, errors.New("请选择项目目录")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return Detection{}, errors.New("项目目录不存在")
	}
	d := Detection{Name: suggestedProjectName(directory)}
	if exists(filepath.Join(directory, "pom.xml")) {
		d.Kind = "spring-maven"
		d.Modules = bootModules(directory)
		if len(d.Modules) == 1 && d.Modules[0] != "." {
			d.Module = d.Modules[0]
		}
		return d, nil
	}
	if exists(filepath.Join(directory, "build.gradle")) || exists(filepath.Join(directory, "build.gradle.kts")) {
		d.Kind = "spring-gradle"
		return d, nil
	}
	data, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return d, nil
		}
		return Detection{}, err
	}
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return Detection{}, err
	}
	d.Kind, d.PackageManager, d.PortMode = "node", "npm", "none"
	if exists(filepath.Join(directory, "pnpm-lock.yaml")) {
		d.PackageManager = "pnpm"
	} else if exists(filepath.Join(directory, "yarn.lock")) {
		d.PackageManager = "yarn"
	}
	if _, ok := pkg.Dependencies["vite"]; ok {
		d.PortMode = "vite"
	}
	if _, ok := pkg.DevDependencies["vite"]; ok {
		d.PortMode = "vite"
	}
	if _, ok := pkg.Dependencies["@vue/cli-service"]; ok {
		d.PortMode = "vue-cli"
	}
	if _, ok := pkg.DevDependencies["@vue/cli-service"]; ok {
		d.PortMode = "vue-cli"
	}
	for script := range pkg.Scripts {
		d.Scripts = append(d.Scripts, script)
	}
	sort.Strings(d.Scripts)
	for _, script := range []string{"dev:vite", "dev", "serve", "start"} {
		if command, ok := pkg.Scripts[script]; ok {
			d.Script = script
			if strings.Contains(command, "vite") {
				d.PortMode = "vite"
			} else if strings.Contains(command, "vue-cli-service") {
				d.PortMode = "vue-cli"
			}
			break
		}
	}
	return d, nil
}

// A linked worktree has a .git file at its root; a normal checkout has a .git
// directory. Starting from a nested module still gives the worktree's name.
func suggestedProjectName(directory string) string {
	path, err := filepath.Abs(directory)
	if err != nil {
		return filepath.Base(filepath.Clean(directory))
	}
	for candidate := path; ; candidate = filepath.Dir(candidate) {
		if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			return filepath.Base(candidate)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return filepath.Base(path)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
