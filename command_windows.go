//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func managedCommand(spec commandSpec) (*exec.Cmd, error) {
	executable := spec.Executable
	if !strings.ContainsAny(executable, `/\`) {
		// Resolve against the launch environment, which may override PATH.
		var err error
		executable, err = commandOnPath(executable, spec.Env)
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(executable, spec.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	ext := strings.ToLower(filepath.Ext(executable))
	if ext == ".cmd" || ext == ".bat" {
		// cmd.exe uses different quoting rules from CommandLineToArgvW.
		// The outer quotes keep a batch path containing spaces intact.
		args := append([]string{executable}, spec.Args...)
		quoted := make([]string, len(args))
		for i, arg := range args {
			if strings.ContainsAny(arg, "\r\n\x00") {
				return nil, fmt.Errorf("启动参数包含换行或空字符")
			}
			quoted[i] = quoteBatchArgument(arg)
		}
		comspec := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		cmd = exec.Command(comspec)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true,
			CmdLine: syscall.EscapeArg(comspec) + ` /d /v:off /s /c "` + strings.Join(quoted, " ") + `"`,
		}
	}
	cmd.Dir, cmd.Env = spec.Directory, spec.Env
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

func commandOnPath(name string, env []string) (string, error) {
	path := environmentValue(env, "PATH")
	for _, directory := range filepath.SplitList(path) {
		if directory == "" {
			continue
		}
		candidate := filepath.Join(strings.Trim(directory, `"`), name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("找不到启动文件 %s；请配置工具路径或 PATH", name)
}

func environmentValue(env []string, name string) string {
	value := os.Getenv(name)
	for _, entry := range env {
		if key, replacement, ok := strings.Cut(entry, "="); ok && strings.EqualFold(key, name) {
			value = replacement
		}
	}
	return value
}

// Force quotes even for arguments containing only shell metacharacters.
// Backslash escaping is for the Java/Node executable invoked by the batch.
func quoteBatchArgument(arg string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	slashes := 0
	for _, char := range arg {
		if char == '\\' {
			slashes++
			continue
		}
		if char == '"' {
			quoted.WriteString(strings.Repeat("\\", slashes*2+1))
		} else {
			quoted.WriteString(strings.Repeat("\\", slashes))
		}
		slashes = 0
		quoted.WriteRune(char)
	}
	quoted.WriteString(strings.Repeat("\\", slashes*2))
	quoted.WriteByte('"')
	return quoted.String()
}
