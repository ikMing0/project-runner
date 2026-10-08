//go:build windows

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type sourceFile struct {
	Size     int64
	Modified time.Time
	Digest   string
}
type sourceSnapshot struct {
	Root  string
	Files map[string]sourceFile
}

// Observe production inputs only. Frontend HMR, tests and build output do not
// make a running backend out of date.
func projectSourceFiles(p Project) (string, map[string]bool, error) {
	root := p.Directory
	files := map[string]bool{}
	if p.Kind == "spring-maven" {
		var err error
		root, _, err = mavenLayout(p)
		if err != nil {
			return "", nil, err
		}
		modules, err := mavenModules(root)
		if err != nil {
			return "", nil, err
		}
		for _, module := range modules {
			files[filepath.Join(module, "pom.xml")] = true
			if err := collectBuildFiles(filepath.Join(module, "src", "main"), files); err != nil {
				return "", nil, err
			}
		}
		if err := collectBuildFiles(filepath.Join(root, ".mvn"), files); err != nil {
			return "", nil, err
		}
		for _, name := range []string{"mvnw", "mvnw.cmd", "settings.xml"} {
			if exists(filepath.Join(root, name)) {
				files[filepath.Join(root, name)] = true
			}
		}
	} else {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch strings.ToLower(entry.Name()) {
				case ".git", ".gradle", "target", "build", "out", "node_modules", ".idea":
					return filepath.SkipDir
				}
				if path != root && exists(filepath.Join(path, "package.json")) {
					return filepath.SkipDir
				}
				if entry.Name() == "src" {
					if err := collectBuildFiles(filepath.Join(path, "main"), files); err != nil {
						return err
					}
					return filepath.SkipDir
				}
				return nil
			}
			switch entry.Name() {
			case "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties", "gradlew", "gradlew.bat", "gradle-wrapper.properties", "gradle-wrapper.jar":
				files[path] = true
			}
			return nil
		})
		if err != nil {
			return "", nil, err
		}
	}
	return root, files, nil
}

func snapshotProjectSources(p Project, previous *sourceSnapshot, full bool) (*sourceSnapshot, error) {
	root, files, err := projectSourceFiles(p)
	if err != nil {
		return nil, err
	}
	if len(files) > 20000 {
		return nil, fmt.Errorf("生产源码文件超过 20000 个，无法完整检查")
	}
	out := &sourceSnapshot{Root: root, Files: map[string]sourceFile{}}
	var bytes int64
	for path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("不能检查特殊源码文件：%s", path)
		}
		bytes += info.Size()
		if info.Size() > 32*1024*1024 || bytes > 512*1024*1024 {
			return nil, fmt.Errorf("生产源码或资源过大，无法完整检查")
		}
		key, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		value := sourceFile{Size: info.Size(), Modified: info.ModTime()}
		cached, found := sourceFile{}, false
		if previous != nil {
			cached, found = previous.Files[key]
		}
		if !full && found && cached.Size == value.Size && cached.Modified.Equal(value.Modified) {
			value.Digest = cached.Digest
		} else {
			value.Digest, err = fileDigest(path)
			if err != nil {
				return nil, err
			}
			after, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
				return nil, fmt.Errorf("源码正在写入，稍后重新检查")
			}
		}
		out.Files[key] = value
	}
	return out, nil
}

func changedSourceMessage(baseline, current *sourceSnapshot) string {
	var paths []string
	for key, old := range baseline.Files {
		value, ok := current.Files[key]
		if !ok || value.Digest != old.Digest {
			paths = append(paths, key)
		}
	}
	for key := range current.Files {
		if _, ok := baseline.Files[key]; !ok {
			paths = append(paths, key)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	return fmt.Sprintf("%d 个生产源码 / 资源 / 构建文件有变化：%s", len(paths), strings.Join(paths[:min(3, len(paths))], "、"))
}

func (a *App) setSourceState(id string, r *run, generation uint64, message, checkError string) {
	a.mu.Lock()
	status := a.statuses[id]
	if a.runs[id] != r || r.stage.Load() != generation ||
		(status.State != "starting" && status.State != "unready" && status.State != "running") {
		a.mu.Unlock()
		return
	}
	changed := message != ""
	if status.SourceChanged == changed && status.SourceMessage == message && status.SourceError == checkError {
		a.mu.Unlock()
		return
	}
	status.SourceChanged, status.SourceMessage, status.SourceError = changed, message, checkError
	a.statuses[id] = status
	a.emitStatus(status)
	a.mu.Unlock()
}

func (a *App) watchProjectSources(id string, p Project, r *run, done <-chan struct{}, generation uint64, baseline *sourceSnapshot, baselineError string, interval time.Duration) {
	if baseline == nil {
		a.setSourceState(id, r, generation, "", "未能取得启动前源码快照，下次重启后重新检查："+baselineError)
		return
	}
	previous := baseline
	lastFull := time.Time{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if r.stage.Load() != generation {
			return
		}
		full := time.Since(lastFull) >= 30*time.Second
		current, err := snapshotProjectSources(p, previous, full)
		if err != nil {
			a.setSourceState(id, r, generation, changedSourceMessage(baseline, previous), "源码检查暂不可用："+err.Error())
		} else {
			if full {
				lastFull = time.Now()
			}
			previous = current
			a.setSourceState(id, r, generation, changedSourceMessage(baseline, current), "")
		}
		select {
		case <-done:
			return
		case <-r.done:
			return
		case <-ticker.C:
		}
	}
}
