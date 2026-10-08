//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	goruntime "runtime"
	"sort"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ProcessMetrics struct {
	ID           string  `json:"id"`
	StartedAt    int64   `json:"startedAt"`
	CPUPercent   float64 `json:"cpuPercent"`
	CPUSampled   bool    `json:"cpuSampled"`
	MemoryBytes  uint64  `json:"memoryBytes"`
	ProcessCount int     `json:"processCount"`
	Partial      bool    `json:"partial"`
	Error        string  `json:"error"`
}
type cpuSample struct {
	Run   *run
	Ticks int64
	At    time.Time
}
type processMemoryCounters struct {
	Size, PageFaultCount                     uint32
	PeakWorkingSet, WorkingSet               uintptr
	QuotaPeakPagedPool, QuotaPagedPool       uintptr
	QuotaPeakNonPagedPool, QuotaNonPagedPool uintptr
	PagefileUsage, PeakPagefileUsage         uintptr
}

var processMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

func jobProcessIDs(job windows.Handle) ([]uint32, error) {
	capacity := 32
	for capacity <= 4096 {
		data := make([]byte, 8+capacity*int(unsafe.Sizeof(uintptr(0))))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&data[0])), uint32(len(data)), nil)
		if err == windows.ERROR_MORE_DATA {
			capacity *= 2
			continue
		}
		if err != nil {
			return nil, err
		}
		count := int(binary.LittleEndian.Uint32(data[4:8]))
		if count > capacity {
			return nil, fmt.Errorf("子进程列表变化，请稍后刷新")
		}
		out := make([]uint32, 0, count)
		for i := 0; i < count; i++ {
			offset := 8 + i*int(unsafe.Sizeof(uintptr(0)))
			out = append(out, binary.LittleEndian.Uint32(data[offset:offset+4]))
		}
		return out, nil
	}
	return nil, fmt.Errorf("子进程过多，无法完整读取")
}

func normalizedCPU(previous cpuSample, r *run, ticks int64, at time.Time, cores int) (float64, bool) {
	if previous.Run != r || previous.At.IsZero() || ticks < previous.Ticks || !at.After(previous.At) || cores < 1 {
		return 0, false
	}
	value := float64(ticks-previous.Ticks) * 100 / (at.Sub(previous.At).Seconds() * 1e7 * float64(cores))
	return min(100, max(0, value)), true
}

// Metrics cover the managed Job's descendants, including cmd/npm/Java children.
// Sampling is requested by the visible UI, without a permanent polling process.
func (a *App) GetProcessMetrics() []ProcessMetrics {
	a.mu.Lock()
	runs := make(map[string]*run, len(a.runs))
	for id, r := range a.runs {
		runs[id] = r
	}
	a.mu.Unlock()
	a.metricsMu.Lock()
	defer a.metricsMu.Unlock()
	if a.cpuSamples == nil {
		a.cpuSamples = map[string]cpuSample{}
	}
	for id := range a.cpuSamples {
		if runs[id] == nil {
			delete(a.cpuSamples, id)
		}
	}
	out := make([]ProcessMetrics, 0, len(runs))
	for id, r := range runs {
		value := ProcessMetrics{ID: id, StartedAt: r.started.UnixMilli()}
		r.mu.Lock()
		if r.job == 0 {
			r.mu.Unlock()
			continue
		}
		var accounting jobAccounting
		err := windows.QueryInformationJobObject(r.job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
		if err != nil {
			value.Error = "无法读取进程 CPU"
			r.mu.Unlock()
			out = append(out, value)
			continue
		}
		now, ticks := time.Now(), accounting.TotalKernelTime+accounting.TotalUserTime
		value.CPUPercent, value.CPUSampled = normalizedCPU(a.cpuSamples[id], r, ticks, now, goruntime.NumCPU())
		a.cpuSamples[id] = cpuSample{r, ticks, now}
		pids, err := jobProcessIDs(r.job)
		if err != nil {
			value.Error = "无法读取子进程内存"
			r.mu.Unlock()
			out = append(out, value)
			continue
		}
		value.ProcessCount = len(pids)
		for _, pid := range pids {
			handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				if err != windows.ERROR_INVALID_PARAMETER {
					value.Partial = true
				}
				continue
			}
			info := processMemoryCounters{Size: uint32(unsafe.Sizeof(processMemoryCounters{}))}
			ok, _, _ := processMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&info)), uintptr(info.Size))
			windows.CloseHandle(handle)
			if ok == 0 {
				value.Partial = true
				continue
			}
			value.MemoryBytes += uint64(info.WorkingSet)
		}
		r.mu.Unlock()
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
