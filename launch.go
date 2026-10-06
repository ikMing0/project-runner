package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type commandSpec struct {
	Executable string
	Args       []string
	Directory  string
	Env        []string
	Temporary  []string
}

func buildCommand(p Project) (commandSpec, error) {
	spec := commandSpec{Directory: p.Directory, Env: os.Environ()}
	for key, value := range p.Environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return spec, fmt.Errorf("无效环境变量: %q", key)
		}
		spec.Env = append(spec.Env, key+"="+value)
	}
	if p.Kind != "node" && p.JavaHome != "" {
		if !exists(filepath.Join(p.JavaHome, "bin", "java.exe")) {
			return spec, fmt.Errorf("JDK 路径无效: %s", p.JavaHome)
		}
		spec.Env = append(spec.Env, "JAVA_HOME="+p.JavaHome)
		spec.Env = append(spec.Env, "PATH="+filepath.Join(p.JavaHome, "bin")+";"+environmentValue(spec.Env, "PATH"))
	}
	if p.Kind == "node" {
		if p.NodeHome != "" {
			if !exists(filepath.Join(p.NodeHome, "node.exe")) {
				return spec, fmt.Errorf("Node.js 目录无效: %s", p.NodeHome)
			}
			spec.Env = append(spec.Env, "PATH="+p.NodeHome+";"+environmentValue(spec.Env, "PATH"))
		}
		manager := p.PackageManager
		if manager == "" {
			manager = "npm"
		}
		if manager != "npm" && manager != "pnpm" && manager != "yarn" {
			return spec, fmt.Errorf("不支持的包管理器: %s", manager)
		}
		spec.Executable = manager + ".cmd"
		if p.ToolPath != "" {
			spec.Executable = p.ToolPath
		}
		spec.Args = []string{"run", p.Script}
		if manager == "npm" {
			spec.Args = append(spec.Args, "--")
		}
		if p.PortMode == "vite" || p.PortMode == "vue-cli" {
			spec.Args = append(spec.Args, "--port", strconv.Itoa(p.Port))
			if p.PortMode == "vite" {
				spec.Args = append(spec.Args, "--strictPort")
			}
		} else if p.PortMode == "env" {
			spec.Env = append(spec.Env, "PORT="+strconv.Itoa(p.Port))
		} else if p.PortMode != "none" {
			return spec, fmt.Errorf("不支持的端口模式: %s", p.PortMode)
		}
		spec.Args = append(spec.Args, splitLines(p.AppArgs)...)
		return spec, nil
	}
	props := append([]string{}, splitLines(p.JVMArgs)...)
	props = append(props, "-Dserver.port="+strconv.Itoa(p.Port))
	if p.ConfigFile != "" {
		if !validPropertyName(p.ConfigProperty) {
			return spec, fmt.Errorf("无效的配置属性名: %s", p.ConfigProperty)
		}
		props = append(props, "-D"+p.ConfigProperty+"="+p.ConfigFile)
	}
	appArgs := splitLines(p.AppArgs)
	if p.Kind == "spring-maven" {
		wrapper := findUp(p.Directory, "mvnw.cmd")
		if p.ToolPath != "" {
			spec.Executable = p.ToolPath
		} else if wrapper != "" {
			spec.Executable = wrapper
		} else {
			spec.Executable = "mvn.cmd"
		}
		if p.Module != "" {
			module, err := safeModule(p.Directory, p.Module)
			if err != nil {
				return spec, err
			}
			spec.Args = append(spec.Args, "-f", filepath.Join(module, "pom.xml"))
		}
		spec.Args = append(spec.Args, "spring-boot:run", "-Dspring-boot.run.jvmArguments="+joinArguments(props))
		if len(appArgs) != 0 {
			spec.Args = append(spec.Args, "-Dspring-boot.run.arguments="+joinArguments(appArgs))
		}
		return spec, nil
	}
	if p.Kind == "spring-gradle" {
		wrapper := findUp(p.Directory, "gradlew.bat")
		if p.ToolPath != "" {
			spec.Executable = p.ToolPath
		} else if wrapper != "" {
			spec.Executable = wrapper
		} else {
			spec.Executable = "gradle.bat"
		}
		initFile, err := os.CreateTemp("", "project-runner-*.gradle")
		if err != nil {
			return spec, err
		}
		path := initFile.Name()
		// Groovy init scripts configure the application task without editing the repository.
		quoted := make([]string, 0, len(props))
		for _, arg := range props {
			quoted = append(quoted, groovyString(arg))
		}
		content := "gradle.projectsEvaluated { allprojects { tasks.matching { it.name == 'bootRun' }.configureEach { t -> t.jvmArgs(" + strings.Join(quoted, ", ") + ") } } }\n"
		if _, err := initFile.WriteString(content); err != nil {
			_ = initFile.Close()
			_ = os.Remove(path)
			return spec, err
		}
		if err := initFile.Close(); err != nil {
			_ = os.Remove(path)
			return spec, err
		}
		spec.Temporary = append(spec.Temporary, path)
		task := "bootRun"
		if p.Module != "" {
			module, err := safeModule(p.Directory, p.Module)
			if err != nil {
				_ = os.Remove(path)
				return spec, err
			}
			rel, err := filepath.Rel(p.Directory, module)
			if err != nil {
				_ = os.Remove(path)
				return spec, err
			}
			task = ":" + strings.ReplaceAll(rel, string(filepath.Separator), ":") + ":bootRun"
		}
		spec.Args = []string{"--no-daemon", "--init-script", path, task}
		if len(appArgs) != 0 {
			spec.Args = append(spec.Args, "--args="+joinArguments(appArgs))
		}
		return spec, nil
	}
	return spec, fmt.Errorf("不支持的启动类型: %s", p.Kind)
}

func splitLines(value string) []string {
	parts := []string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return parts
}

func joinArguments(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			arg = "\"" + strings.ReplaceAll(arg, "\"", "\\\"") + "\""
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func groovyString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "\\'") + "'"
}

func validPropertyName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func safeModule(root, module string) (string, error) {
	path := filepath.Join(root, module)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("模块必须位于项目目录内")
	}
	if !exists(abs) {
		return "", fmt.Errorf("模块目录不存在: %s", module)
	}
	return abs, nil
}

func findUp(start, name string) string {
	for dir := start; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, name)
		if exists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
	}
}
