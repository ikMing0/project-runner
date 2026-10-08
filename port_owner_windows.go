//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/windows"
)

type PortOwner struct {
	PID         uint32 `json:"pid"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Address     string `json:"address"`
	ServiceID   string `json:"serviceId"`
	ServiceName string `json:"serviceName"`
}

var extendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
var processInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func tcpListeners(family uint32) ([]PortOwner, []int, error) {
	size := uint32(0)
	code, _, _ := extendedTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(family), 3, 0)
	if code != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || size < 4 || size > 16*1024*1024 {
		return nil, nil, fmt.Errorf("读取端口信息失败: %d", code)
	}
	for attempt := 0; attempt < 3; attempt++ {
		data := make([]byte, size)
		code, _, _ = extendedTCPTable.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), 3, 0)
		if code == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && size <= 16*1024*1024 {
			continue
		}
		if code != 0 {
			return nil, nil, fmt.Errorf("读取端口信息失败: %d", code)
		}
		stride, portOffset, pidOffset := 24, 8, 20
		if family == windows.AF_INET6 {
			stride, portOffset, pidOffset = 56, 20, 52
		}
		count := int(binary.LittleEndian.Uint32(data[:4]))
		if count > (len(data)-4)/stride {
			return nil, nil, fmt.Errorf("端口表长度无效")
		}
		owners, ports := make([]PortOwner, 0, count), make([]int, 0, count)
		for i := 0; i < count; i++ {
			row := data[4+i*stride : 4+(i+1)*stride]
			port := int(binary.BigEndian.Uint16(row[portOffset : portOffset+2]))
			address := "IPv4"
			if family == windows.AF_INET6 {
				address = "IPv6"
			}
			owners = append(owners, PortOwner{PID: binary.LittleEndian.Uint32(row[pidOffset : pidOffset+4]), Address: address})
			ports = append(ports, port)
		}
		return owners, ports, nil
	}
	return nil, nil, fmt.Errorf("端口表频繁变化，请重新检查")
}

func (a *App) portOwners(port int) []PortOwner {
	owners := []PortOwner{}
	seen := map[uint32]bool{}
	for _, family := range []uint32{windows.AF_INET, windows.AF_INET6} {
		rows, ports, err := tcpListeners(family)
		if err != nil {
			continue
		}
		for i, owner := range rows {
			if ports[i] != port || seen[owner.PID] {
				continue
			}
			seen[owner.PID] = true
			handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, owner.PID)
			if err == nil {
				buffer, size := make([]uint16, 32768), uint32(32768)
				if windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size) == nil {
					owner.Path = windows.UTF16ToString(buffer[:size])
					owner.Name = filepath.Base(owner.Path)
				}
				a.mu.Lock()
				for id, running := range a.runs {
					running.mu.Lock()
					if running.job == 0 {
						running.mu.Unlock()
						continue
					}
					var inside int32
					ok, _, _ := processInJob.Call(uintptr(handle), uintptr(running.job), uintptr(unsafe.Pointer(&inside)))
					running.mu.Unlock()
					if ok != 0 && inside != 0 {
						owner.ServiceID = id
						if p, found := a.projectLocked(id); found {
							owner.ServiceName = p.Name
						}
						break
					}
				}
				a.mu.Unlock()
				windows.CloseHandle(handle)
			}
			if owner.Name == "" {
				owner.Name = "进程信息不可读取"
			}
			owners = append(owners, owner)
		}
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].PID < owners[j].PID })
	return owners
}
