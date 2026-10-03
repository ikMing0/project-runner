//go:build windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type run struct {
	cmd       *exec.Cmd
	started   time.Time
	job       windows.Handle
	done      chan struct{}
	stopOnce  sync.Once
	stopping  bool
	mu        sync.Mutex
	temporary []string
}

func (r *run) stop() {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.stopping = true
		r.mu.Unlock()
		// Hidden console processes have no console to receive CTRL_BREAK.
		_ = windows.TerminateJobObject(r.job, 1)
	})
}

func (a *App) StartProject(id string) error {
	started := time.Now()
	a.mu.Lock()
	p, found := a.projects[id]
	if !found {
		a.mu.Unlock()
		return errors.New("项目不存在")
	}
	if _, active := a.runs[id]; active {
		a.mu.Unlock()
		return errors.New("项目已经在运行")
	}
	usesPort := p.Kind != "node" || p.PortMode != "none"
	for otherID := range a.runs {
		other := a.projects[otherID]
		if usesPort && (other.Kind != "node" || other.PortMode != "none") && otherID != id && other.Port == p.Port {
			a.mu.Unlock()
			return fmt.Errorf("端口 %d 已被另一个运行实例使用", p.Port)
		}
	}
	if usesPort {
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(p.Port))
		if err != nil {
			a.mu.Unlock()
			return fmt.Errorf("端口 %d 已被占用: %w", p.Port, err)
		}
		_ = listener.Close()
	}
	spec, err := buildCommand(p)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	cmd := exec.Command(spec.Executable, spec.Args...)
	cmd.Dir = spec.Directory
	cmd.Env = spec.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.mu.Unlock()
		cleanTemporary(spec.Temporary)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.mu.Unlock()
		cleanTemporary(spec.Temporary)
		return err
	}
	job, err := newJob()
	if err != nil {
		a.mu.Unlock()
		cleanTemporary(spec.Temporary)
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		a.mu.Unlock()
		cleanTemporary(spec.Temporary)
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = windows.CloseHandle(job)
		a.mu.Unlock()
		cleanTemporary(spec.Temporary)
		return fmt.Errorf("无法管理子进程: %w", err)
	}
	r := &run{cmd: cmd, started: started, job: job, done: make(chan struct{}), temporary: spec.Temporary}
	a.runs[id] = r
	a.history[id] = nil
	status := Status{ID: id, State: "starting", PID: cmd.Process.Pid, StartedAt: started.UnixMilli()}
	a.statuses[id] = status
	a.mu.Unlock()
	a.emitStatus(status)
	a.appendLog(id, "system", fmt.Sprintf("启动 %s（PID %d，端口 %d）", p.Name, cmd.Process.Pid, p.Port))
	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); a.readOutput(id, stdout, "stdout") }()
	go func() { defer readers.Done(); a.readOutput(id, stderr, "stderr") }()
	if p.Kind == "node" && p.PortMode == "none" {
		a.markRunning(id, r)
	} else {
		go a.waitForReady(id, p.Port, r)
	}
	go func() {
		err := cmd.Wait()
		// The launcher may finish before its child. The job ensures descendants exit.
		_ = windows.TerminateJobObject(job, 1)
		readers.Wait()
		waitForJobExit(job)
		if usesPort {
			waitForPortRelease(p.Port)
		}
		_ = windows.CloseHandle(job)
		cleanTemporary(r.temporary)
		status := Status{ID: id, State: "stopped", PID: 0}
		r.mu.Lock()
		requested := r.stopping
		r.mu.Unlock()
		if err != nil && !requested {
			status.State = "failed"
			status.Error = err.Error()
		}
		if cmd.ProcessState != nil {
			exit := cmd.ProcessState.ExitCode()
			status.ExitCode = &exit
		}
		a.mu.Lock()
		previous := a.statuses[id]
		status.StartedAt = previous.StartedAt
		status.StartupDurationMs = previous.StartupDurationMs
		delete(a.runs, id)
		a.statuses[id] = status
		a.mu.Unlock()
		a.appendLog(id, "system", "进程已退出")
		a.emitStatus(status)
		close(r.done)
	}()
	return nil
}

type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func waitForJobExit(job windows.Handle) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		var info jobAccounting
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
		if err != nil || info.ActiveProcesses == 0 || time.Now().After(deadline) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForPortRelease(port int) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			_ = listener.Close()
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func newJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func (a *App) waitForReady(id string, port int, r *run) {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 150*time.Millisecond)
			if err != nil {
				continue
			}
			_ = conn.Close()
			a.markRunning(id, r)
			return
		}
	}
}

func (a *App) markRunning(id string, r *run) {
	a.mu.Lock()
	status := a.statuses[id]
	if a.runs[id] != r || status.State != "starting" {
		a.mu.Unlock()
		return
	}
	duration := time.Since(r.started).Milliseconds()
	status.State = "running"
	status.StartupDurationMs = &duration
	a.statuses[id] = status
	a.mu.Unlock()
	a.emitStatus(status)
}

func (a *App) readOutput(id string, pipe io.Reader, source string) {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !utf8.Valid(line) {
			if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(line); err == nil {
				line = decoded
			}
		}
		a.appendLog(id, source, string(line))
	}
	if err := scanner.Err(); err != nil {
		a.appendLog(id, "system", "读取日志失败: "+err.Error())
	}
}

func (a *App) appendLog(id, source, message string) {
	line := LogLine{Time: time.Now().Format("15:04:05"), Source: source, Level: classifyLogLevel(source, message), Text: message}
	a.mu.Lock()
	a.history[id] = append(a.history[id], line)
	if len(a.history[id]) > 2500 {
		a.history[id] = append([]LogLine{}, a.history[id][500:]...)
	}
	a.mu.Unlock()
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "project:log", struct {
		ID   string  `json:"id"`
		Line LogLine `json:"line"`
	}{id, line})
}

func (a *App) emitStatus(status Status) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "project:status", status)
	}
}

func (a *App) StopProject(id string) error {
	a.mu.Lock()
	r := a.runs[id]
	a.mu.Unlock()
	if r == nil {
		return errors.New("项目未运行")
	}
	r.stop()
	return nil
}

func (a *App) RestartProject(id string) error {
	a.mu.Lock()
	r := a.runs[id]
	a.mu.Unlock()
	if r != nil {
		r.stop()
		<-r.done
	}
	return a.StartProject(id)
}

func cleanTemporary(files []string) {
	for _, file := range files {
		_ = os.Remove(file)
	}
}
