//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

type EditorSettings struct {
	IDEAPath     string `json:"ideaPath"`
	DetectedPath string `json:"detectedPath"`
}

func (a *App) editorPath() (string, error) {
	a.mu.Lock()
	config := a.configPath
	a.mu.Unlock()
	if config == "" {
		return "", fmt.Errorf("无法确定编辑器设置目录")
	}
	return filepath.Join(filepath.Dir(config), "editor.json"), nil
}
func validIDEAExecutable(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base != "idea64.exe" && base != "idea.exe" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func (a *App) GetEditorSettings(projectID string) (EditorSettings, error) {
	a.editorMu.Lock()
	defer a.editorMu.Unlock()
	return a.editorSettings(projectID)
}
func (a *App) editorSettings(projectID string) (EditorSettings, error) {
	path, err := a.editorPath()
	if err != nil {
		return EditorSettings{}, err
	}
	var settings EditorSettings
	if err = readBoundedJSON(path, &settings, 16384); err != nil && !os.IsNotExist(err) {
		return settings, err
	}
	a.mu.Lock()
	p, _ := a.projectLocked(projectID)
	if parent, ok := a.projects[strings.TrimSuffix(projectID, ":frontend")]; ok && strings.HasSuffix(projectID, ":frontend") {
		p.ToolPath = parent.ToolPath
	}
	a.mu.Unlock()
	settings.DetectedPath = discoverIDEA(p.ToolPath)
	return settings, nil
}
func (a *App) SetIDEAPath(path string) error {
	a.editorMu.Lock()
	defer a.editorMu.Unlock()
	path = strings.TrimSpace(path)
	if path != "" {
		var err error
		path, err = filepath.Abs(path)
		if err != nil || !validIDEAExecutable(path) {
			return fmt.Errorf("请选择 IDEA 安装目录 bin 下的 idea64.exe 或 idea.exe")
		}
	}
	config, err := a.editorPath()
	if err != nil {
		return err
	}
	return privateJSON(config, EditorSettings{IDEAPath: path})
}
func (a *App) PickIDEAPath() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("文件选择器尚未就绪")
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择 IDEA 启动程序（bin/idea64.exe）",
		Filters: []runtime.FileFilter{{DisplayName: "IDEA 启动程序", Pattern: "idea64.exe;idea.exe"}}})
}

func discoverIDEA(tool string) string {
	// IDEA's bundled Maven path provides the installation without machine-specific constants.
	if tool != "" {
		for dir := filepath.Dir(tool); ; dir = filepath.Dir(dir) {
			for _, name := range []string{"idea64.exe", "idea.exe"} {
				path := filepath.Join(dir, "bin", name)
				if validIDEAExecutable(path) {
					return path
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	for _, name := range []string{"idea64.exe", "idea.exe"} {
		if path, err := exec.LookPath(name); err == nil && validIDEAExecutable(path) {
			return path
		}
	}
	// Detect a currently open IDEA, including installations on another drive.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err == nil {
		defer windows.CloseHandle(snapshot)
		entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
		for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
			name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
			if name != "idea64.exe" && name != "idea.exe" {
				continue
			}
			handle, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if openErr != nil {
				continue
			}
			buffer, size := make([]uint16, 32768), uint32(32768)
			queryErr := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
			windows.CloseHandle(handle)
			if queryErr == nil {
				path := windows.UTF16ToString(buffer[:size])
				if validIDEAExecutable(path) {
					return path
				}
			}
		}
	}
	var candidates []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base == "" {
			continue
		}
		for _, name := range []string{"idea64.exe", "idea.exe"} {
			found, _ := filepath.Glob(filepath.Join(base, "JetBrains", "IntelliJ IDEA*", "bin", name))
			candidates = append(candidates, found...)
		}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		if validIDEAExecutable(candidates[i]) {
			return candidates[i]
		}
	}
	return ""
}

func sourceWorktreeRoot(p Project) string {
	if marker := findUp(p.Directory, ".git"); marker != "" {
		return filepath.Dir(marker)
	}
	if p.Kind == "spring-maven" {
		if root, _, err := mavenLayout(p); err == nil {
			return root
		}
	}
	return p.Directory
}
func containedSource(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
func sourceLocation(p Project, path string, line, column int) (string, []string, error) {
	if line < 1 || line > 10000000 || column < 0 || column > 1000000 {
		return "", nil, fmt.Errorf("源码行列号无效")
	}
	path = strings.TrimSpace(path)
	if strings.ContainsAny(path, "\x00\r\n") || strings.HasPrefix(path, "\\\\") || strings.Contains(path, "://") {
		return "", nil, fmt.Errorf("源码路径无效")
	}
	// Maven emits /D:/... on Windows.
	if len(path) > 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	path = filepath.FromSlash(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".java", ".kt", ".kts", ".groovy", ".scala", ".js", ".jsx", ".ts", ".tsx", ".vue", ".xml":
	default:
		return "", nil, fmt.Errorf("仅支持打开源码文件")
	}
	if !filepath.IsAbs(path) {
		if filepath.VolumeName(path) != "" || strings.HasPrefix(path, "\\") {
			return "", nil, fmt.Errorf("源码路径需要完整盘符")
		}
		relative := path
		path = filepath.Join(p.Directory, relative)
		if !exists(path) && p.Module != "" {
			path = filepath.Join(p.Directory, p.Module, relative)
		}
	}
	root, err := filepath.EvalSymlinks(sourceWorktreeRoot(p))
	if err != nil {
		return "", nil, fmt.Errorf("无法确定当前工作树：%w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", nil, fmt.Errorf("源码文件不存在：%w", err)
	}
	root, _ = filepath.Abs(root)
	resolved, _ = filepath.Abs(resolved)
	if !containedSource(root, resolved) {
		return "", nil, fmt.Errorf("该源码不在当前项目工作树内")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("源码文件不是普通文件")
	}
	args := []string{"--line", strconv.Itoa(line)}
	if column > 0 {
		args = append(args, "--column", strconv.Itoa(column))
	}
	args = append(args, resolved)
	return resolved, args, nil
}
func (a *App) OpenSourceInIDEA(serviceID, path string, line, column int) error {
	a.mu.Lock()
	p, ok := a.projectLocked(serviceID)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("运行配置不存在")
	}
	resolved, args, err := sourceLocation(p, path, line, column)
	if err != nil {
		return err
	}
	settings, err := a.GetEditorSettings(serviceID)
	if err != nil {
		return err
	}
	launcher := settings.IDEAPath
	if launcher == "" {
		launcher = settings.DetectedPath
	}
	if !validIDEAExecutable(launcher) {
		return fmt.Errorf("IDEA_NOT_FOUND: 请在「IDEA 跳转设置」中选择 idea64.exe")
	}
	cmd := exec.Command(launcher, args...)
	cmd.Dir = filepath.Dir(resolved)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("打开 IDEA 失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
