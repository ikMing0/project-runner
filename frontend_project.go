package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FrontendConfig struct {
	Directory      string            `json:"directory"`
	Port           int               `json:"port"`
	PackageManager string            `json:"packageManager"`
	Script         string            `json:"script"`
	PortMode       string            `json:"portMode"`
	NodeHome       string            `json:"nodeHome"`
	ToolPath       string            `json:"toolPath"`
	AppArgs        string            `json:"appArgs"`
	Environment    map[string]string `json:"environment"`
	AutoProxy      bool              `json:"autoProxy"`
	ProxyVariable  string            `json:"proxyVariable"`
}

func frontendID(id string) string { return id + ":frontend" }

func frontendProject(backend Project) Project {
	f := backend.Frontend
	variable := f.ProxyVariable
	if variable == "" {
		variable = "VUE_APP_BASE_API_TARGET"
	}
	env := make(map[string]string, len(f.Environment)+2)
	for key, value := range f.Environment {
		// Windows environment names are case-insensitive. Remove aliases before
		// applying managed values so map iteration cannot undo a port/proxy.
		if (f.AutoProxy && strings.EqualFold(key, variable)) || (f.PortMode != "none" && strings.EqualFold(key, "port")) {
			continue
		}
		env[key] = value
	}
	if f.AutoProxy {
		env[variable] = fmt.Sprintf("http://localhost:%d", backend.Port)
	}
	// This Vue project's config also reads the lowercase port variable.
	if f.PortMode != "none" {
		env["port"] = fmt.Sprint(f.Port)
	}
	return Project{ID: frontendID(backend.ID), Name: backend.Name + " · 前端", Kind: "node", Directory: f.Directory,
		Port: f.Port, PackageManager: f.PackageManager, Script: f.Script, PortMode: f.PortMode,
		NodeHome: f.NodeHome, ToolPath: f.ToolPath, AppArgs: f.AppArgs, Environment: env}
}

// Caller holds a.mu. Child services are derived, not saved as duplicate projects.
func (a *App) projectLocked(id string) (Project, bool) {
	if p, ok := a.projects[id]; ok {
		return p, true
	}
	parentID, child := strings.CutSuffix(id, ":frontend")
	if child {
		if p, ok := a.projects[parentID]; ok && p.Frontend != nil {
			return frontendProject(p), true
		}
	}
	return Project{}, false
}

func validateFrontend(f FrontendConfig, backendPort int) (FrontendConfig, error) {
	dir, err := filepath.Abs(f.Directory)
	if strings.TrimSpace(f.Directory) == "" || err != nil {
		return f, errors.New("请选择前端项目目录")
	}
	f.Directory = filepath.Clean(dir)
	if f.Port < 1 || f.Port > 65535 {
		return f, errors.New("前端端口必须在 1–65535 之间")
	}
	if f.PortMode == "" {
		f.PortMode = "vite"
	}
	if f.PortMode != "vite" && f.PortMode != "vue-cli" && f.PortMode != "env" && f.PortMode != "none" {
		return f, errors.New("不支持的前端端口传入方式")
	}
	if f.PortMode != "none" && f.Port == backendPort {
		return f, errors.New("前端和后端必须使用不同端口")
	}
	if f.PackageManager == "" {
		f.PackageManager = "npm"
	}
	if f.PackageManager != "npm" && f.PackageManager != "pnpm" && f.PackageManager != "yarn" {
		return f, errors.New("不支持的前端包管理器")
	}
	data, err := os.ReadFile(filepath.Join(f.Directory, "package.json"))
	if err != nil {
		return f, fmt.Errorf("无法读取前端 package.json: %w", err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return f, fmt.Errorf("前端 package.json 无效: %w", err)
	}
	if _, ok := pkg.Scripts[f.Script]; !ok {
		return f, fmt.Errorf("前端 package.json 中没有脚本 %q", f.Script)
	}
	if f.NodeHome != "" {
		f.NodeHome, err = filepath.Abs(f.NodeHome)
		if err != nil || !exists(filepath.Join(f.NodeHome, "node.exe")) {
			return f, errors.New("前端 Node.js 目录无效，需要包含 node.exe")
		}
	}
	if f.ToolPath != "" {
		f.ToolPath, err = filepath.Abs(f.ToolPath)
		info, statErr := os.Stat(f.ToolPath)
		if err != nil || statErr != nil || info.IsDir() {
			return f, errors.New("前端包管理器启动文件不存在")
		}
	}
	if f.ProxyVariable == "" {
		f.ProxyVariable = "VUE_APP_BASE_API_TARGET"
	}
	if f.AutoProxy && (strings.ContainsAny(f.ProxyVariable, "=\x00\r\n ") || f.ProxyVariable == "") {
		return f, errors.New("前端代理环境变量名无效")
	}
	for key, value := range f.Environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return f, fmt.Errorf("无效前端环境变量: %q", key)
		}
	}
	return f, nil
}

// Locate common frontend directories inside the selected checkout. A user may
// still browse to any explicit directory when projects use a different layout.
func (a *App) DetectFrontend(directory string) (FrontendConfig, error) {
	if strings.TrimSpace(directory) == "" {
		return FrontendConfig{}, errors.New("请先选择项目或前端目录")
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return FrontendConfig{}, err
	}
	for dir := filepath.Clean(abs); ; dir = filepath.Dir(dir) {
		for _, name := range []string{"", "ruoyi-ui", "frontend", "web", "client"} {
			candidate := filepath.Join(dir, name)
			if !exists(filepath.Join(candidate, "package.json")) {
				continue
			}
			detection, err := a.DetectProject(candidate)
			if err != nil {
				return FrontendConfig{}, err
			}
			if detection.Kind != "node" {
				continue
			}
			script := detection.Script
			if script == "" && len(detection.Scripts) > 0 {
				script = detection.Scripts[0]
			}
			f := FrontendConfig{Directory: candidate, Port: 82, PackageManager: detection.PackageManager,
				Script: script, PortMode: detection.PortMode, AutoProxy: true, ProxyVariable: "VUE_APP_BASE_API_TARGET"}
			if node, err := exec.LookPath("node.exe"); err == nil {
				f.NodeHome = filepath.Dir(node)
			}
			if tool, err := exec.LookPath(f.PackageManager + ".cmd"); err == nil {
				f.ToolPath = tool
			}
			return f, nil
		}
		if exists(filepath.Join(dir, ".git")) || filepath.Dir(dir) == dir {
			break
		}
	}
	return FrontendConfig{}, errors.New("没有找到前端 package.json，请选择前端项目目录")
}
