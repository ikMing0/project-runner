//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

type StartupCheck struct {
	ServiceID   string      `json:"serviceId"`
	ServiceName string      `json:"serviceName"`
	Label       string      `json:"label"`
	State       string      `json:"state"`
	Detail      string      `json:"detail"`
	Owners      []PortOwner `json:"owners"`
}
type StartupReport struct {
	Allowed bool           `json:"allowed"`
	Checks  []StartupCheck `json:"checks"`
}
type toolVersionResult struct {
	Text string
	Time time.Time
}

var toolVersions sync.Map
var toolVersionNumber = regexp.MustCompile(`(?i)(?:version[\s"]*|v)(\d+(?:\.\d+){0,3})`)

type versionOutput struct{ bytes.Buffer }

func (b *versionOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 16384 {
		_, _ = b.Buffer.Write(p[:min(n, 16384-b.Len())])
	}
	return n, nil
}
func probeToolVersion(spec commandSpec) string {
	key := spec.Executable + "|" + environmentValue(spec.Env, "JAVA_HOME") + "|" + environmentValue(spec.Env, "PATH")
	if cached, found := toolVersions.Load(key); found && time.Since(cached.(toolVersionResult).Time) < 5*time.Minute {
		return cached.(toolVersionResult).Text
	}
	cmd, err := managedCommand(spec)
	if err != nil {
		return "无法查询版本"
	}
	job, err := newJob()
	if err != nil {
		return "无法查询版本"
	}
	defer windows.CloseHandle(job)
	var stdout, stderr versionOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err = cmd.Start(); err != nil {
		return "无法查询版本"
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "无法查询版本"
	}
	timeoutDone := make(chan struct{})
	timer := time.AfterFunc(2*time.Second, func() { _ = windows.TerminateJobObject(job, 1); close(timeoutDone) })
	err = cmd.Wait()
	if !timer.Stop() {
		<-timeoutDone
	}
	_ = windows.TerminateJobObject(job, 1)
	output := stdout.String() + "\n" + stderr.String()
	text := "版本查询超时或工具未返回版本"
	if err == nil {
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if toolVersionNumber.MatchString(line) || strings.Contains(line, "Apache Maven") || strings.HasPrefix(line, "Gradle ") || regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(line) {
				text = redactCodexLog(line)
				if len(text) > 240 {
					text = text[:240]
				}
				break
			}
		}
	}
	toolVersions.Store(key, toolVersionResult{text, time.Now()})
	return text
}

func preflightEnvironment(p Project) []string {
	env := append([]string{}, os.Environ()...)
	for key, value := range p.Environment {
		env = append(env, key+"="+value)
	}
	home := p.JavaHome
	if p.Kind == "node" {
		home = p.NodeHome
	}
	if home != "" {
		path := home
		if p.Kind != "node" {
			env = append(env, "JAVA_HOME="+home)
			path = filepath.Join(home, "bin")
		}
		env = append(env, "PATH="+path+";"+environmentValue(env, "PATH"))
	}
	return env
}

func (a *App) CheckProject(p Project, side string) (StartupReport, error) {
	services := []Project{p}
	if side == "frontend" {
		if p.Frontend == nil {
			return StartupReport{}, fmt.Errorf("没有配套前端")
		}
		services = []Project{frontendProject(p)}
	} else if side == "" && p.Frontend != nil {
		services = append(services, frontendProject(p))
	}
	if side != "" && side != "backend" && side != "frontend" {
		return StartupReport{}, fmt.Errorf("检查范围无效")
	}
	return a.checkServices(services, true), nil
}

func (a *App) checkServices(services []Project, versions bool) StartupReport {
	report := StartupReport{Allowed: true, Checks: []StartupCheck{}}
	ports := map[int]bool{}
	for _, p := range services {
		add := func(label, state, detail string, owners ...PortOwner) {
			report.Checks = append(report.Checks, StartupCheck{p.ID, p.Name, label, state, redactCodexLog(detail), append([]PortOwner{}, owners...)})
			if state == "error" {
				report.Allowed = false
			}
		}
		if _, err := validateHealth(p.Health); err != nil {
			add("就绪检查", "error", err.Error())
		}
		for key := range p.Environment {
			if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00\r\n") {
				add("环境变量", "error", "环境变量名称不能为空或包含等号、换行")
				break
			}
		}
		if info, err := os.Stat(p.Directory); strings.TrimSpace(p.Directory) == "" || err != nil || !info.IsDir() {
			add("项目目录", "error", "目录不存在，请重新选择")
		} else {
			add("项目目录", "ok", p.Directory)
		}
		env := preflightEnvironment(p)
		checkTool := func(label, name string, args []string) string {
			path := name
			var err error
			if !strings.ContainsAny(path, "/\\") {
				path, err = commandOnPath(path, env)
			}
			info, statErr := os.Stat(path)
			if err != nil || statErr != nil || info.IsDir() {
				add(label, "error", "找不到 "+name+"，请配置工具目录或 PATH")
				return ""
			}
			detail := path
			state := "ok"
			if versions {
				// Wrapper version commands may download runtimes before showing a
				// version. Report that without executing the project wrapper.
				version := "使用项目 Wrapper（启动时按项目版本执行）"
				if !strings.EqualFold(filepath.Base(path), "mvnw.cmd") && !strings.EqualFold(filepath.Base(path), "gradlew.bat") {
					version = probeToolVersion(commandSpec{Executable: path, Args: args, Directory: p.Directory, Env: env})
				}
				detail += " · " + version
				if strings.Contains(version, "无法查询") || strings.Contains(version, "未返回版本") {
					state = "warn"
				}
			}
			add(label, state, detail)
			return path
		}
		if p.Kind == "node" {
			checkTool("Node.js", "node.exe", []string{"--version"})
			manager := p.PackageManager
			if manager == "" {
				manager = "npm"
			}
			tool := p.ToolPath
			if tool == "" {
				tool = manager + ".cmd"
			}
			checkTool("包管理器", tool, []string{"--version"})
			var pkg struct {
				Scripts         map[string]string `json:"scripts"`
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
				Engines         map[string]string `json:"engines"`
			}
			data, err := os.ReadFile(filepath.Join(p.Directory, "package.json"))
			if err != nil || json.Unmarshal(data, &pkg) != nil {
				add("启动脚本", "error", "无法读取有效的 package.json")
			} else {
				if _, ok := pkg.Scripts[p.Script]; !ok || p.Script == "" {
					add("启动脚本", "error", "package.json 中不存在脚本 "+p.Script)
				} else {
					add("启动脚本", "ok", p.Script)
				}
				if len(pkg.Dependencies)+len(pkg.DevDependencies) > 0 && !exists(filepath.Join(p.Directory, "node_modules")) && !exists(filepath.Join(p.Directory, ".pnp.cjs")) {
					add("前端依赖", "error", "缺少依赖目录，请先在项目终端安装依赖")
				} else {
					add("前端依赖", "ok", "依赖目录已存在，或脚本没有声明依赖")
				}
				if pkg.Engines["node"] != "" {
					add("Node 版本要求", "info", pkg.Engines["node"])
				}
			}
		} else {
			java := "java.exe"
			if home := environmentValue(env, "JAVA_HOME"); home != "" {
				java = filepath.Join(home, "bin", "java.exe")
			}
			checkTool("Java", java, []string{"-version"})
			tool := p.ToolPath
			if p.Kind == "spring-maven" {
				if tool == "" {
					tool = findUp(p.Directory, "mvnw.cmd")
					if tool == "" {
						tool = "mvn.cmd"
					}
				}
				checkTool("Maven", tool, []string{"--version"})
				_, module, err := mavenLayout(p)
				if err != nil {
					add("启动模块", "error", err.Error())
				} else {
					add("启动模块", "ok", module)
					pom, _ := readMavenPOM(module)
					for _, property := range pom.Properties.Items {
						if property.XMLName.Local == "java.version" || property.XMLName.Local == "maven.compiler.release" {
							add("Java 版本要求", "info", strings.TrimSpace(property.Value))
							break
						}
					}
				}
			} else if p.Kind == "spring-gradle" {
				if tool == "" {
					tool = findUp(p.Directory, "gradlew.bat")
					if tool == "" {
						tool = "gradle.bat"
					}
				}
				checkTool("Gradle", tool, []string{"--version"})
				if !exists(filepath.Join(p.Directory, "build.gradle")) && !exists(filepath.Join(p.Directory, "build.gradle.kts")) {
					add("构建文件", "error", "缺少 build.gradle 或 build.gradle.kts")
				}
			} else {
				add("启动类型", "error", "不支持的启动类型")
			}
			if p.ConfigFile != "" {
				if _, err := os.Stat(p.ConfigFile); err != nil {
					add("外部配置", "error", "配置文件或目录不存在")
				} else {
					add("外部配置", "ok", p.ConfigFile)
				}
			}
		}
		dependencies, dependencyErr := validateDependencies(p.Dependencies)
		if dependencyErr != nil {
			add("依赖服务", "error", dependencyErr.Error())
		} else {
			for _, result := range probeDependencies(dependencies) {
				state, detail := "ok", "TCP 连接成功（未检查认证或数据库内容）"
				if result.Error != nil {
					state, detail = "warn", "TCP 连接未成功，请确认服务、地址和网络"
					if result.Check.Required {
						state = "error"
					}
				}
				d := result.Check
				add("依赖 · "+d.Name, state, fmt.Sprintf("%s:%d · %s", d.Host, d.Port, detail))
			}
		}
		a.mu.Lock()
		active := a.runs[p.ID] != nil
		a.mu.Unlock()
		if active {
			add("服务状态", "info", "当前服务已运行，端口由运行台管理")
		} else if p.Kind != "node" || p.PortMode != "none" {
			if p.Port < 1 || p.Port > 65535 {
				add("端口", "error", "端口必须在 1–65535 之间")
			} else if ports[p.Port] {
				add("端口", "error", fmt.Sprintf("前后端重复使用端口 %d", p.Port))
			} else if err := checkPortAvailable(p.Port); err != nil {
				add("端口", "error", fmt.Sprintf("端口 %d 已被占用: %s", p.Port, err), a.portOwners(p.Port)...)
			} else {
				add("端口", "ok", fmt.Sprintf("%d 可用", p.Port))
			}
			ports[p.Port] = true
		}
	}
	return report
}

func (a *App) checkSavedServices(ids []string) error {
	return a.checkConfiguredServices(ids, false)
}

func (a *App) checkConfiguredServices(ids []string, includeActive bool) error {
	a.mu.Lock()
	var services []Project
	for _, id := range ids {
		if (a.runs[id] != nil || a.frontendWaits[id] != nil) && !includeActive {
			continue
		}
		if p, ok := a.projectLocked(id); ok {
			services = append(services, p)
		}
	}
	a.mu.Unlock()
	report := a.checkServices(services, false)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "project:preflight", report)
	}
	if !report.Allowed {
		var messages []string
		for _, check := range report.Checks {
			if check.State == "error" {
				messages = append(messages, check.ServiceName+" · "+check.Label+": "+check.Detail)
			}
		}
		return fmt.Errorf("启动检查未通过：\n%s", strings.Join(messages, "\n"))
	}
	return nil
}
