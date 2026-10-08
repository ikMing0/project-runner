//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
)

func (a *App) serviceIDs(id string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, found := a.projectLocked(id)
	if !found {
		return nil, errors.New("项目不存在")
	}
	ids := []string{id}
	if p.Frontend != nil {
		ids = append(ids, frontendID(id))
	}
	return ids, nil
}

// Group control is serialized so stop/restart cannot race the second launch.
// Backend builds and frontend dev servers run independently after spawning.
func (a *App) StartProject(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	if a.closing {
		return errors.New("运行台正在关闭")
	}
	ids, err := a.serviceIDs(id)
	if err != nil {
		return err
	}
	return a.startServices(ids, false)
}

func (a *App) StartService(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	if a.closing {
		return errors.New("运行台正在关闭")
	}
	a.mu.Lock()
	waiting := a.frontendWaits[id] != nil
	a.mu.Unlock()
	if waiting {
		return errors.New("前端正在等待后端，可停止等待后单独启动")
	}
	if err := a.checkSavedServices([]string{id}); err != nil {
		return err
	}
	return a.startProject(id, false)
}

func (a *App) StopService(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	if a.cancelFrontendWaits([]string{id}, "用户停止，取消前端等待") && strings.HasSuffix(id, ":frontend") {
		return nil
	}
	return a.stopService(id)
}

func (a *App) StopProject(id string) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	ids, err := a.serviceIDs(id)
	if err != nil {
		return err
	}
	if !a.stopServices(ids, false) {
		return errors.New("项目未运行")
	}
	return nil
}

func (a *App) RestartProject(id string) error { return a.restartGroup(id, false, false) }

func (a *App) RebuildProject(id string) error { return a.restartGroup(id, true, false) }

func (a *App) RestartService(id string) error { return a.restartGroup(id, false, true) }

func (a *App) restartGroup(id string, clean, single bool) error {
	a.groupMu.Lock()
	defer a.groupMu.Unlock()
	if a.closing {
		return errors.New("运行台正在关闭")
	}
	ids, err := a.serviceIDs(id)
	if err != nil {
		return err
	}
	if single {
		ids = []string{id}
	}
	if clean {
		a.mu.Lock()
		p, _ := a.projectLocked(id)
		a.mu.Unlock()
		if p.Kind != "spring-maven" {
			return errors.New("重新构建仅适用于 Maven 项目")
		}
	}
	// Validate settings before stopping a healthy instance. Ports held by the
	// same active services are intentionally skipped until after they exit.
	if err := a.checkConfiguredServices(ids, true); err != nil {
		return err
	}
	a.stopServices(ids, true)
	return a.startServices(ids, clean)
}

func (a *App) stopServices(ids []string, wait bool) bool {
	cancelled := a.cancelFrontendWaits(ids, "用户停止，取消前端等待")
	a.mu.Lock()
	var runs []*run
	for _, id := range ids {
		if r := a.runs[id]; r != nil {
			runs = append(runs, r)
		}
	}
	a.mu.Unlock()
	for _, r := range runs {
		r.stop()
	}
	if wait {
		for _, r := range runs {
			<-r.done
		}
	}
	return len(runs) > 0 || cancelled
}

func (a *App) startServices(ids []string, clean bool) error {
	if err := a.checkSavedServices(ids); err != nil {
		return err
	}
	// Check both ports before starting either side. Existing services are left
	// running when the user starts only the missing side of a partial group.
	a.mu.Lock()
	var pending []string
	for _, id := range ids {
		if a.runs[id] == nil && a.frontendWaits[id] == nil {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		a.mu.Unlock()
		return errors.New("项目已经在运行")
	}
	ports := map[int]bool{}
	for _, id := range pending {
		p, _ := a.projectLocked(id)
		if p.Frontend != nil {
			if _, err := validateFrontend(*p.Frontend, p.Port); err != nil {
				a.mu.Unlock()
				return err
			}
		}
		if p.Kind == "node" {
			spec, err := buildCommand(p)
			if err == nil {
				_, err = managedCommand(spec)
			}
			if err != nil {
				a.mu.Unlock()
				return fmt.Errorf("%s: %w", p.Name, err)
			}
		}
		if p.Kind == "node" && p.PortMode == "none" {
			continue
		}
		if ports[p.Port] {
			a.mu.Unlock()
			return errors.New("前端和后端必须使用不同端口")
		}
		ports[p.Port] = true
		for otherID := range a.runs {
			other, _ := a.projectLocked(otherID)
			if (other.Kind != "node" || other.PortMode != "none") && other.Port == p.Port {
				a.mu.Unlock()
				return fmt.Errorf("%s 的端口 %d 已被另一个运行实例使用", p.Name, p.Port)
			}
		}
		if err := checkPortAvailable(p.Port); err != nil {
			a.mu.Unlock()
			return fmt.Errorf("%s 的端口 %d 已被占用: %w", p.Name, p.Port, err)
		}
	}
	a.mu.Unlock()
	var started []string
	for _, id := range pending {
		if a.scheduleFrontendWait(id) {
			continue
		}
		if err := a.startProject(id, clean); err != nil {
			a.stopServices(started, true)
			return err
		}
		started = append(started, id)
	}
	return nil
}
