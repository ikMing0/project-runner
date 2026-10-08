//go:build windows

package main

import (
	"fmt"
	"os"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type trayWindowClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type trayIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}
type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          trayPoint
	Private        uint32
}
type windowsTray struct {
	window         atomic.Uintptr
	available      atomic.Bool
	closing        atomic.Bool
	done           chan struct{}
	icon           trayIconData
	taskbarMessage uint32
	open, quit     func()
	availability   func(bool)
}

var trayUser = windows.NewLazySystemDLL("user32.dll")
var trayShell = windows.NewLazySystemDLL("shell32.dll")
var trayKernel = windows.NewLazySystemDLL("kernel32.dll")
var trayRegister = trayUser.NewProc("RegisterClassExW")
var trayUnregister = trayUser.NewProc("UnregisterClassW")
var trayCreate = trayUser.NewProc("CreateWindowExW")
var trayDestroy = trayUser.NewProc("DestroyWindow")
var trayDefault = trayUser.NewProc("DefWindowProcW")
var trayPost = trayUser.NewProc("PostMessageW")
var trayGetMessage = trayUser.NewProc("GetMessageW")
var trayDispatch = trayUser.NewProc("DispatchMessageW")
var trayPostQuit = trayUser.NewProc("PostQuitMessage")
var trayLoadImage = trayUser.NewProc("LoadImageW")
var trayDestroyIcon = trayUser.NewProc("DestroyIcon")
var trayNotify = trayShell.NewProc("Shell_NotifyIconW")
var trayWindows sync.Map
var trayNumber atomic.Uint64

const trayCallbackMessage = 0x8001

// Wails packages the app's RT_GROUP_ICON under resource ID 3.
const applicationIconResource = 3

func loadTrayIcon(instance uintptr) (icon uintptr, owned bool) {
	width, _, _ := trayUser.NewProc("GetSystemMetrics").Call(49)  // SM_CXSMICON
	height, _, _ := trayUser.NewProc("GetSystemMetrics").Call(50) // SM_CYSMICON
	if width == 0 {
		width = 16
	}
	if height == 0 {
		height = 16
	}
	// Load the exact small size; shared icon caching can return a larger bitmap.
	icon, _, _ = trayLoadImage.Call(instance, applicationIconResource, 1, width, height, 0)
	if icon != 0 {
		return icon, true
	}
	icon, _, _ = trayLoadImage.Call(0, 32512, 1, width, height, 0x8000)
	return icon, false // Shared system fallback for unpackaged builds.
}

var trayProcedure = windows.NewCallback(func(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
	if value, ok := trayWindows.Load(window); ok {
		t := value.(*windowsTray)
		if t.handleMessage(message, lparam) {
			return 0
		}
		if message == 0x0010 {
			trayDestroy.Call(window)
			return 0
		} // WM_CLOSE
		if message == 0x0002 {
			trayPostQuit.Call(0)
			return 0
		} // WM_DESTROY
	}
	result, _, _ := trayDefault.Call(window, uintptr(message), wparam, lparam)
	return result
})

func newWindowsTray(open, quit func(), availability func(bool)) *windowsTray {
	return &windowsTray{done: make(chan struct{}), open: open, quit: quit, availability: availability}
}
func (t *windowsTray) setAvailable(value bool) {
	if t.available.Swap(value) != value && t.availability != nil {
		t.availability(value)
	}
}
func (t *windowsTray) addIcon() bool {
	ok, _, _ := trayNotify.Call(0, uintptr(unsafe.Pointer(&t.icon))) // NIM_ADD
	t.setAvailable(ok != 0)
	return ok != 0
}
func (t *windowsTray) handleMessage(message uint32, lparam uintptr) bool {
	if message == t.taskbarMessage && message != 0 {
		if !t.addIcon() && !t.closing.Load() && t.open != nil {
			go t.open()
		}
		return true
	}
	if message != trayCallbackMessage {
		return false
	}
	switch uint32(lparam) {
	case 0x0202, 0x0203, 0x0400, 0x0401: // left release/double click, keyboard select
		if t.open != nil {
			go t.open()
		}
	case 0x0205, 0x007b:
		t.showMenu()
	}
	return true
}
func (t *windowsTray) showMenu() {
	menu, _, _ := trayUser.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer trayUser.NewProc("DestroyMenu").Call(menu)
	appendItem := func(id uintptr, label string) {
		text, _ := windows.UTF16PtrFromString(label)
		trayUser.NewProc("AppendMenuW").Call(menu, 0, id, uintptr(unsafe.Pointer(text)))
	}
	appendItem(1, "打开运行台")
	appendItem(2, "退出并停止所有项目")
	var point trayPoint
	trayUser.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	window := t.window.Load()
	trayUser.NewProc("SetForegroundWindow").Call(window)
	command, _, _ := trayUser.NewProc("TrackPopupMenu").Call(menu, 0x0100|0x0002, uintptr(point.X), uintptr(point.Y), 0, window, 0)
	trayPost.Call(window, 0, 0, 0) // WM_NULL releases popup menu ownership.
	if command == 1 && t.open != nil {
		go t.open()
	}
	if command == 2 && t.quit != nil {
		go t.quit()
	}
}
func (t *windowsTray) start() error {
	ready := make(chan error, 1)
	go t.loop(ready)
	return <-ready
}
func (t *windowsTray) loop(ready chan<- error) {
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	defer close(t.done)
	defer t.setAvailable(false)
	defer func() {
		var msg trayMessage
		trayUser.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0x0012, 0x0012, 1)
	}()
	instance, _, _ := trayKernel.NewProc("GetModuleHandleW").Call(0)
	className, _ := windows.UTF16PtrFromString(fmt.Sprintf("ProjectRunnerTray_%d_%d", os.Getpid(), trayNumber.Add(1)))
	class := trayWindowClass{Size: uint32(unsafe.Sizeof(trayWindowClass{})), Procedure: trayProcedure, Instance: instance, ClassName: className}
	atom, _, err := trayRegister.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		ready <- fmt.Errorf("无法注册托盘窗口：%v", err)
		return
	}
	defer trayUnregister.Call(uintptr(unsafe.Pointer(className)), instance)
	title, _ := windows.UTF16PtrFromString("项目运行台")
	// A hidden top-level window receives TaskbarCreated after Explorer restarts.
	window, _, err := trayCreate.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		ready <- fmt.Errorf("无法创建托盘窗口：%v", err)
		return
	}
	t.window.Store(window)
	trayWindows.Store(window, t)
	defer func() { trayWindows.Delete(window); t.window.Store(0) }()
	defer trayDestroy.Call(window)
	taskbar, _ := windows.UTF16PtrFromString("TaskbarCreated")
	message, _, _ := trayUser.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(taskbar)))
	t.taskbarMessage = uint32(message)
	icon, owned := loadTrayIcon(instance)
	if owned {
		defer trayDestroyIcon.Call(icon)
	}
	t.icon = trayIconData{Size: uint32(unsafe.Sizeof(trayIconData{})), Window: window, ID: 1, Flags: 1 | 2 | 4, Callback: trayCallbackMessage, Icon: icon}
	tip, _ := windows.UTF16FromString("项目运行台 · 点击打开，右键退出")
	copy(t.icon.Tip[:], tip)
	if !t.addIcon() {
		trayDestroy.Call(window)
		ready <- fmt.Errorf("系统未能创建托盘图标")
		return
	}
	defer trayNotify.Call(2, uintptr(unsafe.Pointer(&t.icon))) // NIM_DELETE
	ready <- nil
	var msg trayMessage
	for {
		result, _, _ := trayGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		trayDispatch.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if !t.closing.Load() && t.open != nil {
		go t.open()
	}
}
func (t *windowsTray) close() {
	if t.closing.Swap(true) {
		return
	}
	if window := t.window.Load(); window != 0 {
		trayPost.Call(window, 0x0010, 0, 0)
	}
	select {
	case <-t.done:
	case <-time.After(time.Second):
	}
}
