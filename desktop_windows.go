//go:build windows

package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type DesktopSettings struct {
	CloseToTray   bool `json:"closeToTray"`
	TrayAvailable bool `json:"trayAvailable"`
}

func (a *App) desktopPath() (string, error) {
	a.mu.Lock()
	path := a.configPath
	a.mu.Unlock()
	if path == "" {
		return "", fmt.Errorf("无法确定应用设置目录")
	}
	return filepath.Join(filepath.Dir(path), "desktop.json"), nil
}
func (a *App) loadDesktopSettings() {
	path, err := a.desktopPath()
	if err != nil {
		return
	}
	var settings DesktopSettings
	if readBoundedJSON(path, &settings, 16384) == nil {
		a.desktopMu.Lock()
		a.desktop.CloseToTray = settings.CloseToTray
		a.desktopMu.Unlock()
	}
}
func (a *App) GetDesktopSettings() DesktopSettings {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	settings := a.desktop
	settings.TrayAvailable = a.tray != nil && a.tray.available.Load()
	return settings
}
func (a *App) SaveDesktopSettings(settings DesktopSettings) error {
	path, err := a.desktopPath()
	if err != nil {
		return err
	}
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if err = privateJSON(path, DesktopSettings{CloseToTray: settings.CloseToTray}); err != nil {
		return err
	}
	a.desktop.CloseToTray = settings.CloseToTray
	return nil
}
func (a *App) domReady(ctx context.Context) {
	tray := newWindowsTray(a.showWindow, func() { _ = a.QuitApplication() }, func(bool) {
		runtime.EventsEmit(ctx, "desktop:settings", a.GetDesktopSettings())
	})
	if err := tray.start(); err != nil {
		runtime.EventsEmit(ctx, "desktop:tray-error", "托盘不可用，关闭窗口仍会退出并停止所有项目")
		return
	}
	a.desktopMu.Lock()
	a.tray = tray
	a.desktopMu.Unlock()
	runtime.EventsEmit(ctx, "desktop:settings", a.GetDesktopSettings())
}
func (a *App) showWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
	}
}
func (a *App) HideToTray() error {
	if a.ctx == nil || !a.GetDesktopSettings().TrayAvailable {
		return fmt.Errorf("托盘暂不可用，窗口保持打开")
	}
	runtime.WindowHide(a.ctx)
	return nil
}
func closeShouldHide(settings DesktopSettings, quitting bool) bool {
	return settings.CloseToTray && settings.TrayAvailable && !quitting
}
func (a *App) beforeClose(ctx context.Context) bool {
	if closeShouldHide(a.GetDesktopSettings(), a.forceQuit.Load()) {
		if a.HideToTray() == nil {
			return true
		}
	}
	return false
}
func (a *App) QuitApplication() error {
	if a.ctx == nil {
		return fmt.Errorf("运行台尚未就绪")
	}
	a.forceQuit.Store(true)
	runtime.Quit(a.ctx)
	return nil
}
func (a *App) closeTray() {
	a.desktopMu.Lock()
	tray := a.tray
	a.tray = nil
	a.desktopMu.Unlock()
	if tray != nil {
		tray.close()
	}
}
