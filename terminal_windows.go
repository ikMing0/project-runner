//go:build windows

package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const terminalHistoryLimit = 1024 * 1024
const terminalSessionLimit = 16

type TerminalInfo struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	Shell     string `json:"shell"`
	State     string `json:"state"`
	PID       int    `json:"pid"`
	ExitCode  *int   `json:"exitCode"`
}

// ConPTY emits UTF-8 bytes, which can split a character between reads. Base64
// transports the original bytes through JSON without replacing partial runes.
type TerminalOutput struct {
	ID       string `json:"id"`
	Sequence uint64 `json:"sequence"`
	Data     string `json:"data"`
}

type TerminalSnapshot struct {
	Info      TerminalInfo `json:"info"`
	Sequence  uint64       `json:"sequence"`
	Data      string       `json:"data"`
	Truncated bool         `json:"truncated"`
}

type terminalSession struct {
	mu             sync.Mutex
	consoleMu      sync.Mutex
	writeMu        sync.Mutex
	info           TerminalInfo
	console        windows.Handle
	process        windows.Handle
	job            windows.Handle
	input, output  *os.File
	history        []byte
	sequence       uint64
	truncated      bool
	stopOnce       sync.Once
	done, readDone chan struct{}
	onOutput       func(TerminalOutput)
	onExit         func(TerminalInfo)
	cleanup        func()
}

func terminalSize(cols, rows int) (windows.Coord, error) {
	if cols < 2 || cols > 1000 || rows < 1 || rows > 500 {
		return windows.Coord{}, errors.New("终端大小无效")
	}
	return windows.Coord{X: int16(cols), Y: int16(rows)}, nil
}

func (a *App) NewTerminal(serviceID string, cols, rows int) (TerminalInfo, error) {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	return a.newTerminalLocked(serviceID, cols, rows, "", "", nil)
}

// Caller holds groupMu; Codex shares the same tab and process-tree lifecycle.
func (a *App) newTerminalLocked(serviceID string, cols, rows int, title, command string, cleanup func()) (TerminalInfo, error) {
	if a.closing {
		return TerminalInfo{}, errors.New("运行台正在关闭")
	}
	a.mu.Lock()
	p, found := a.projectLocked(serviceID)
	a.mu.Unlock()
	if !found {
		return TerminalInfo{}, errors.New("请先保存项目配置，再打开终端")
	}
	if info, err := os.Stat(p.Directory); err != nil || !info.IsDir() {
		return TerminalInfo{}, errors.New("终端工作目录不存在")
	}
	a.terminalMu.Lock()
	full := len(a.terminals) >= terminalSessionLimit
	a.terminalMu.Unlock()
	if full {
		return TerminalInfo{}, errors.New("最多打开 16 个终端，请先关闭不需要的标签页")
	}
	size, err := terminalSize(cols, rows)
	if err != nil {
		return TerminalInfo{}, err
	}
	env, err := terminalEnvironment(p)
	if err != nil {
		return TerminalInfo{}, err
	}
	shell, label := terminalShell(env)
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return TerminalInfo{}, err
	}
	a.terminalNumber++
	if title == "" {
		title = fmt.Sprintf("终端 %d", a.terminalNumber)
	} else {
		label = "Codex CLI"
	}
	info := TerminalInfo{ID: hex.EncodeToString(id), ProjectID: strings.TrimSuffix(serviceID, ":frontend"), ServiceID: serviceID,
		Title: title, Directory: p.Directory, Shell: label, State: "running"}
	s, err := startTerminal(info, shell, env, size, func(output TerminalOutput) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "terminal:output", output)
		}
	}, func(info TerminalInfo) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "terminal:exit", info)
		}
	}, command)
	if err != nil {
		return TerminalInfo{}, fmt.Errorf("打开终端失败: %w", err)
	}
	s.cleanup = cleanup
	a.terminalMu.Lock()
	a.terminals[info.ID] = s
	a.terminalMu.Unlock()
	go s.wait()
	return s.snapshot().Info, nil
}

func terminalShell(env []string) (string, string) {
	if shell, err := commandOnPath("pwsh.exe", env); err == nil {
		return shell, "PowerShell 7"
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		system = filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	return filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "PowerShell"
}

func terminalEnvironment(p Project) ([]string, error) {
	env := append([]string{}, os.Environ()...)
	keys := make([]string, 0, len(p.Environment))
	for key := range p.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := p.Environment[key]
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("无效环境变量: %q", key)
		}
		env = append(env, key+"="+value)
	}
	path := environmentValue(env, "PATH")
	if p.JavaHome != "" {
		env = append(env, "JAVA_HOME="+p.JavaHome)
		path = filepath.Join(p.JavaHome, "bin") + ";" + path
	}
	if p.NodeHome != "" {
		path = p.NodeHome + ";" + path
	}
	if p.ToolPath != "" {
		path = filepath.Dir(p.ToolPath) + ";" + path
	}
	env = append(env, "PATH="+path, "TERM=xterm-256color", "COLORTERM=truecolor")
	// CreateProcess requires a sorted, double-NUL-terminated environment block.
	values := map[string]string{}
	for _, entry := range env {
		if key, _, ok := strings.Cut(entry, "="); ok && key != "" {
			values[strings.ToUpper(key)] = entry
		}
	}
	keys = keys[:0]
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env = env[:0]
	for _, key := range keys {
		env = append(env, values[key])
	}
	return env, nil
}

func startTerminal(info TerminalInfo, shell string, env []string, size windows.Coord, output func(TerminalOutput), exit func(TerminalInfo), command ...string) (_ *terminalSession, resultErr error) {
	if err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find(); err != nil {
		return nil, errors.New("内置终端需要 Windows 10 1809 或更新版本")
	}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		windows.CloseHandle(inputRead)
		windows.CloseHandle(inputWrite)
		return nil, err
	}
	s := &terminalSession{info: info, input: os.NewFile(uintptr(inputWrite), "terminal-input"), output: os.NewFile(uintptr(outputRead), "terminal-output"),
		done: make(chan struct{}), readDone: make(chan struct{}), onOutput: output, onExit: exit}
	// Always drain output, including when CreateProcess fails or ConPTY closes.
	go s.readOutput()
	defer func() {
		// Release our copies before waiting for EOF. ConPTY owns its own copies;
		// keeping outputWrite open here would hang a failed process launch.
		windows.CloseHandle(inputRead)
		windows.CloseHandle(outputWrite)
		if resultErr != nil {
			s.stop()
			if s.process != 0 {
				windows.CloseHandle(s.process)
			}
			if s.job != 0 {
				windows.CloseHandle(s.job)
			}
			<-s.readDone
			s.output.Close()
		}
	}()
	if err := windows.CreatePseudoConsole(size, inputRead, outputWrite, 0, &s.console); err != nil {
		return nil, err
	}
	job, err := newJob()
	if err != nil {
		return nil, err
	}
	s.job = job
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	// The pseudo-console attribute takes the handle value, not its address.
	consoleValue := *(*unsafe.Pointer)(unsafe.Pointer(&s.console))
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, consoleValue, unsafe.Sizeof(s.console)); err != nil {
		return nil, err
	}
	// Explicit null standard handles prevent inheriting redirected parent stdio;
	// the child receives its input/output from the attached pseudoconsole instead.
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESHOWWINDOW | windows.STARTF_USESTDHANDLES, ShowWindow: windows.SW_HIDE}, ProcThreadAttributeList: attributes.List()}
	appName, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		return nil, err
	}
	// Initialise text encodings without loading profiles or executing project code.
	initialise := "$OutputEncoding = [Console]::InputEncoding = [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding"
	arguments := []string{shell, "-NoLogo", "-NoProfile", "-NoExit", "-Command", initialise}
	if len(command) > 0 && command[0] != "" {
		// EncodedCommand preserves literal paths, quotes and Unicode. Commands are
		// generated by the launcher, never taken from log text.
		encoded := utf16.Encode([]rune(initialise + "; " + command[0]))
		bytes := make([]byte, len(encoded)*2)
		for i, value := range encoded {
			bytes[2*i], bytes[2*i+1] = byte(value), byte(value>>8)
		}
		arguments = []string{shell, "-NoLogo", "-NoProfile", "-EncodedCommand", base64.StdEncoding.EncodeToString(bytes)}
	}
	args, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(arguments))
	if err != nil {
		return nil, err
	}
	directory, err := windows.UTF16PtrFromString(info.Directory)
	if err != nil {
		return nil, err
	}
	environment := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	var process windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(appName, args, nil, nil, false, flags, &environment[0], directory, &startup.StartupInfo, &process); err != nil {
		return nil, err
	}
	s.process = process.Process
	defer windows.CloseHandle(process.Thread)
	if err := windows.AssignProcessToJobObject(s.job, s.process); err != nil {
		windows.TerminateProcess(s.process, 1)
		return nil, fmt.Errorf("无法管理终端子进程: %w", err)
	}
	if _, err := windows.ResumeThread(process.Thread); err != nil {
		return nil, err
	}
	s.info.PID = int(process.ProcessId)
	return s, nil
}

func (s *terminalSession) readOutput() {
	defer close(s.readDone)
	buffer := make([]byte, 32*1024)
	for {
		count, err := s.output.Read(buffer)
		if count > 0 {
			s.mu.Lock()
			s.sequence++
			s.history = append(s.history, buffer[:count]...)
			if len(s.history) > terminalHistoryLimit {
				s.history = append([]byte{}, s.history[len(s.history)-terminalHistoryLimit:]...)
				s.truncated = true
			}
			chunk := TerminalOutput{ID: s.info.ID, Sequence: s.sequence, Data: base64.StdEncoding.EncodeToString(buffer[:count])}
			s.mu.Unlock()
			s.onOutput(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *terminalSession) snapshot() TerminalSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return TerminalSnapshot{Info: s.info, Sequence: s.sequence, Data: base64.StdEncoding.EncodeToString(s.history), Truncated: s.truncated}
}

func (s *terminalSession) stop() {
	s.stopOnce.Do(func() {
		// Closing ConPTY can emit a final frame and block until it is drained.
		// Never hold the output/history mutex while calling ClosePseudoConsole.
		s.consoleMu.Lock()
		defer s.consoleMu.Unlock()
		if s.job != 0 {
			windows.TerminateJobObject(s.job, 1)
		}
		s.input.Close()
		if s.console != 0 {
			windows.ClosePseudoConsole(s.console)
			s.console = 0
		}
	})
}

func (s *terminalSession) wait() {
	windows.WaitForSingleObject(s.process, windows.INFINITE)
	var code uint32
	windows.GetExitCodeProcess(s.process, &code)
	s.stop()
	<-s.readDone
	s.output.Close()
	windows.CloseHandle(s.process)
	windows.CloseHandle(s.job)
	s.mu.Lock()
	exit := int(code)
	s.info.State = "exited"
	s.info.ExitCode = &exit
	info := s.info
	s.mu.Unlock()
	s.onExit(info)
	if s.cleanup != nil {
		s.cleanup()
	}
	close(s.done)
}

func (a *App) terminal(id string) (*terminalSession, error) {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	s := a.terminals[id]
	if s == nil {
		return nil, errors.New("终端标签页不存在或已关闭")
	}
	return s, nil
}

func (a *App) GetTerminals() []TerminalInfo {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	items := make([]TerminalInfo, 0, len(a.terminals))
	for _, s := range a.terminals {
		items = append(items, s.snapshot().Info)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Title < items[j].Title })
	return items
}

func (a *App) GetTerminalOutput(id string) (TerminalSnapshot, error) {
	s, err := a.terminal(id)
	if err != nil {
		return TerminalSnapshot{}, err
	}
	return s.snapshot(), nil
}

func (a *App) WriteTerminal(id, data string) error {
	if len(data) > 256*1024 {
		return errors.New("单次终端输入过长，请分段粘贴")
	}
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.input.Write([]byte(data))
	return err
}

func (a *App) ResizeTerminal(id string, cols, rows int) error {
	size, err := terminalSize(cols, rows)
	if err != nil {
		return err
	}
	s, err := a.terminal(id)
	if err != nil {
		return err
	}
	s.consoleMu.Lock()
	defer s.consoleMu.Unlock()
	if s.console == 0 {
		return errors.New("终端进程已退出")
	}
	return windows.ResizePseudoConsole(s.console, size)
}

func (a *App) CloseTerminal(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	s, err := a.terminal(id)
	if err != nil {
		return nil
	}
	s.stop()
	<-s.done
	a.terminalMu.Lock()
	delete(a.terminals, id)
	a.terminalMu.Unlock()
	return nil
}

// Caller holds groupMu. Empty projectID closes all terminals on app shutdown.
func (a *App) closeProjectTerminals(projectID string) {
	a.terminalMu.Lock()
	var sessions []*terminalSession
	for id, s := range a.terminals {
		if projectID == "" || s.info.ProjectID == projectID {
			sessions = append(sessions, s)
			delete(a.terminals, id)
		}
	}
	a.terminalMu.Unlock()
	for _, s := range sessions {
		s.stop()
	}
	for _, s := range sessions {
		<-s.done
	}
}
