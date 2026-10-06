//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type run struct {
	cmd            *exec.Cmd
	started        time.Time
	job            windows.Handle
	done           chan struct{}
	stopOnce       sync.Once
	stopping       bool
	mu             sync.Mutex
	temporary      []string
	buildDirectory string
}

var errRunCancelled = errors.New("启动已取消")

func (r *run) stop() {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.stopping = true
		if r.job != 0 {
			_ = windows.TerminateJobObject(r.job, 1)
		}
	})
}

func (a *App) startProject(id string, clean bool) error {
	started := time.Now()
	a.mu.Lock()
	p, found := a.projectLocked(id)
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
		other, _ := a.projectLocked(otherID)
		if usesPort && (other.Kind != "node" || other.PortMode != "none") && otherID != id && other.Port == p.Port {
			a.mu.Unlock()
			return fmt.Errorf("端口 %d 已被另一个运行实例使用", p.Port)
		}
	}
	if usesPort {
		if err := checkPortAvailable(p.Port); err != nil {
			a.mu.Unlock()
			return fmt.Errorf("端口 %d 已被占用: %w", p.Port, err)
		}
	}
	plan, err := buildLaunchPlan(p, clean)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	for _, other := range a.runs {
		if plan.BuildDirectory != "" && strings.EqualFold(plan.BuildDirectory, other.buildDirectory) {
			a.mu.Unlock()
			cleanTemporary(plan.Temporary)
			return errors.New("同一工作树已有构建或运行实例，请先停止该实例")
		}
	}
	job, err := newJob()
	if err != nil {
		a.mu.Unlock()
		cleanTemporary(plan.Temporary)
		return err
	}
	r := &run{started: started, job: job, done: make(chan struct{}), temporary: plan.Temporary, buildDirectory: plan.BuildDirectory}
	a.runs[id], a.history[id] = r, nil
	state := "starting"
	if plan.Cache != nil {
		state = "checking"
	} else if plan.Build != nil {
		state = "building"
	}
	status := Status{ID: id, State: state, StartedAt: started.UnixMilli()}
	a.statuses[id] = status
	a.mu.Unlock()
	a.emitStatus(status)
	go a.runPlan(id, p, r, plan)
	return nil
}

func (a *App) runPlan(id string, p Project, r *run, plan launchPlan) {
	phase := "启动"
	var cmd *exec.Cmd
	var err error
	build := plan.Build
	if plan.Cache != nil {
		phase = "检查"
		a.appendLog(id, "system", "检查当前工作树的构建输入和启动产物")
		needed, reason := plan.Cache.needsBuild()
		a.appendLog(id, "system", reason)
		if !needed {
			build = nil
		} else {
			err = checkMavenArtifactUnlocked(plan.Cache.root, plan.Cache.module)
			if err == nil {
				err = plan.Cache.invalidate()
			}
			if plan.Cache.cleanRequired {
				prepared := *build
				prepared.Args = append([]string{}, build.Args...)
				for i := len(prepared.Args) - 1; i >= 0; i-- {
					if prepared.Args[i] == "package" {
						prepared.Args = append(append(prepared.Args[:i:i], "clean"), prepared.Args[i:]...)
						break
					}
				}
				build = &prepared
			}
		}
	}
	if err == nil && build != nil {
		phase = "构建"
		a.appendLog(id, "system", "构建当前工作树及依赖模块: "+plan.BuildDirectory)
		cmd, err = a.executeStage(id, p, r, *build, "building")
	}
	if err == nil {
		r.mu.Lock()
		cancelled := r.stopping
		r.mu.Unlock()
		if cancelled {
			err = errRunCancelled
		}
	}
	if err == nil {
		phase = "启动"
		var spec commandSpec
		spec, err = plan.Next()
		if err == nil {
			if build != nil {
				if plan.Cache != nil {
					if cacheErr := plan.Cache.record(); cacheErr != nil {
						a.appendLog(id, "system", "未保存构建复用记录，下一次将重新构建: "+cacheErr.Error())
					}
				}
				a.appendLog(id, "system", "构建成功，启动当前工作树的产物")
			}
			cmd, err = a.executeStage(id, p, r, spec, "starting")
		}
	}
	a.finishRun(id, p, r, cmd, phase, err)
}

func (a *App) executeStage(id string, p Project, r *run, spec commandSpec, state string) (*exec.Cmd, error) {
	cmd, err := managedCommand(spec)
	if err != nil {
		return nil, err
	}
	stdout := &logWriter{app: a, id: id, source: "stdout"}
	stderr := &logWriter{app: a, id: id, source: "stderr"}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return nil, errRunCancelled
	}
	if err = cmd.Start(); err != nil {
		r.mu.Unlock()
		return cmd, err
	}
	process, assignErr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if assignErr == nil {
		assignErr = windows.AssignProcessToJobObject(r.job, process)
		_ = windows.CloseHandle(process)
	}
	if assignErr != nil {
		_ = cmd.Process.Kill()
		r.mu.Unlock()
		_ = cmd.Wait()
		stdout.flush()
		stderr.flush()
		return cmd, fmt.Errorf("无法管理子进程: %w", assignErr)
	}
	r.cmd = cmd
	r.mu.Unlock()
	a.mu.Lock()
	status := a.statuses[id]
	status.State, status.PID = state, cmd.Process.Pid
	a.statuses[id] = status
	a.mu.Unlock()
	a.emitStatus(status)
	label := "启动"
	if state == "building" {
		label = "构建"
	}
	a.appendLog(id, "system", fmt.Sprintf("%s %s（PID %d，端口 %d）", label, p.Name, cmd.Process.Pid, p.Port))
	if state == "starting" {
		if p.Kind == "node" && p.PortMode == "none" {
			a.markRunning(id, r)
		} else {
			go a.waitForReady(id, p.Port, r)
		}
	}
	err = cmd.Wait()
	stdout.flush()
	stderr.flush()
	// A batch launcher may exit while descendants still exist.
	r.mu.Lock()
	_ = windows.TerminateJobObject(r.job, 1)
	r.mu.Unlock()
	waitForJobExit(r.job)
	return cmd, err
}

func (a *App) finishRun(id string, p Project, r *run, cmd *exec.Cmd, phase string, err error) {
	r.mu.Lock()
	requested := r.stopping
	_ = windows.TerminateJobObject(r.job, 1)
	waitForJobExit(r.job)
	_ = windows.CloseHandle(r.job)
	r.job = 0
	r.mu.Unlock()
	if p.Kind != "node" || p.PortMode != "none" {
		waitForPortRelease(p.Port)
	}
	cleanTemporary(r.temporary)
	status := Status{ID: id, State: "stopped"}
	if err != nil && !requested {
		status.State = "failed"
		status.Error = phase + "失败: " + err.Error()
		a.appendLog(id, "system", status.Error)
	} else {
		a.appendLog(id, "system", "进程已退出")
	}
	if cmd != nil && cmd.ProcessState != nil {
		exit := cmd.ProcessState.ExitCode()
		status.ExitCode = &exit
	}
	a.mu.Lock()
	previous := a.statuses[id]
	status.StartedAt, status.StartupDurationMs = previous.StartedAt, previous.StartupDurationMs
	a.statuses[id] = status
	// Close done while holding the map lock: a replacement run cannot publish
	// its initial status before the old run's final status has been emitted.
	a.emitStatus(status)
	delete(a.runs, id)
	close(r.done)
	a.mu.Unlock()
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
		if checkPortAvailable(port) == nil {
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

// os/exec owns the copying goroutines and drains them before Wait returns.
// This avoids losing the final error lines when a process exits immediately.
type logWriter struct {
	app        *App
	id, source string
	pending    []byte
}

func (w *logWriter) Write(data []byte) (int, error) {
	count := len(data)
	w.pending = append(w.pending, data...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		w.line(bytes.TrimSuffix(w.pending[:index], []byte("\r")))
		w.pending = w.pending[index+1:]
	}
	if len(w.pending) > 2*1024*1024 {
		w.flush()
	}
	return count, nil
}

func (w *logWriter) line(line []byte) {
	if !utf8.Valid(line) {
		if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(line); err == nil {
			line = decoded
		}
	}
	w.app.appendLog(w.id, w.source, string(line))
}

func (w *logWriter) flush() {
	if len(w.pending) != 0 {
		w.line(w.pending)
		w.pending = nil
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

func (a *App) stopService(id string) error {
	a.mu.Lock()
	r := a.runs[id]
	a.mu.Unlock()
	if r == nil {
		return errors.New("项目未运行")
	}
	r.stop()
	return nil
}

func cleanTemporary(files []string) {
	for _, file := range files {
		_ = os.Remove(file)
	}
}
