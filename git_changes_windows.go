//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

type GitChange struct {
	Path           string `json:"path"`
	OldPath        string `json:"oldPath,omitempty"`
	IndexStatus    string `json:"indexStatus"`
	WorktreeStatus string `json:"worktreeStatus"`
	Staged         bool   `json:"staged"`
	Unstaged       bool   `json:"unstaged"`
	Untracked      bool   `json:"untracked"`
	Conflict       bool   `json:"conflict"`
}

type GitChanges struct {
	Root     string      `json:"root"`
	Branch   string      `json:"branch"`
	Detached bool        `json:"detached"`
	Files    []GitChange `json:"files"`
}

type GitFileDiff struct {
	Text      string `json:"text"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Untracked bool   `json:"untracked"`
}

type gitOutput struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *gitOutput) Write(p []byte) (int, error) {
	n := len(p)
	left := b.limit - b.Len()
	if n > left {
		b.truncated = true
		p = p[:max(0, left)]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// Only fixed, read-only Git commands are exposed. Avoid index refresh writes,
// inherited repository overrides, pagers, fsmonitor hooks and external diffs.
func readGit(directory string, limit int, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	base := []string{"--no-pager", "--literal-pathspecs", "-c", "core.quotepath=false", "-c", "color.ui=false", "-c", "core.fsmonitor=false", "-C", directory}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	stdout := &gitOutput{limit: limit}
	stderr := &gitOutput{limit: 4096}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, false, fmt.Errorf("Git 查询超时，请稍后刷新")
	}
	if err != nil {
		if _, ok := err.(*exec.Error); ok {
			return nil, false, fmt.Errorf("找不到 Git，请将 Git 加入 PATH")
		}
		return nil, false, fmt.Errorf("Git 查询失败：%s", strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), stdout.truncated, nil
}

func parseGitChanges(data []byte) ([]GitChange, error) {
	files := []GitChange{}
	positions := map[string]int{}
	records := bytes.Split(data, []byte{0})
	for i := 0; i < len(records); i++ {
		r := records[i]
		if len(r) == 0 {
			continue
		}
		if len(r) < 4 || r[2] != ' ' {
			return nil, fmt.Errorf("无法解析 Git 文件状态，请刷新重试")
		}
		f := GitChange{Path: string(r[3:]), IndexStatus: string(r[0]), WorktreeStatus: string(r[1])}
		f.Untracked = string(r[:2]) == "??"
		f.Staged = !f.Untracked && r[0] != ' '
		f.Unstaged = !f.Untracked && r[1] != ' '
		f.Conflict = r[0] == 'U' || r[1] == 'U' || string(r[:2]) == "AA" || string(r[:2]) == "DD"
		if r[0] == 'R' || r[0] == 'C' || r[1] == 'R' || r[1] == 'C' {
			i++
			if i >= len(records) || len(records[i]) == 0 {
				return nil, fmt.Errorf("Git 重命名记录不完整")
			}
			f.OldPath = string(records[i])
		}
		// A staged deletion followed by an untracked recreation can have two
		// records for the same path. Keep one file with both available views.
		if j, found := positions[f.Path]; found {
			files[j].Staged = files[j].Staged || f.Staged
			files[j].Unstaged = files[j].Unstaged || f.Unstaged
			files[j].Untracked = files[j].Untracked || f.Untracked
		} else {
			positions[f.Path] = len(files)
			files = append(files, f)
		}
	}
	return files, nil
}

func (a *App) GetGitChanges(directory string) (GitChanges, error) {
	result := GitChanges{Files: []GitChange{}}
	if strings.TrimSpace(directory) == "" {
		return result, fmt.Errorf("请先选择项目目录")
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return result, fmt.Errorf("项目目录不存在")
	}
	root, _, err := readGit(directory, 65536, "rev-parse", "--show-toplevel")
	if err != nil {
		return result, fmt.Errorf("无法读取当前工作树，请确认目录属于 Git 仓库：%w", err)
	}
	result.Root = filepath.Clean(strings.TrimSpace(string(root)))
	branch, _, branchErr := readGit(result.Root, 65536, "symbolic-ref", "--quiet", "--short", "HEAD")
	if branchErr != nil {
		result.Detached = true
		branch, _, err = readGit(result.Root, 65536, "rev-parse", "--short", "HEAD")
		if err != nil {
			return result, err
		}
	}
	result.Branch = strings.TrimSpace(string(branch))
	data, truncated, err := readGit(result.Root, 8*1024*1024, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return result, err
	}
	if truncated {
		return result, fmt.Errorf("未提交文件列表超过 8 MiB，请缩小仓库或调整忽略配置后重试")
	}
	result.Files, err = parseGitChanges(data)
	return result, err
}

func (a *App) GetGitFileDiff(directory, path, view string) (GitFileDiff, error) {
	result := GitFileDiff{}
	if view != "staged" && view != "working" {
		return result, fmt.Errorf("不支持的 Git 查看类型")
	}
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return result, fmt.Errorf("无效的仓库文件路径")
	}
	changes, err := a.GetGitChanges(directory)
	if err != nil {
		return result, err
	}
	var file *GitChange
	for i := range changes.Files {
		if changes.Files[i].Path == path {
			file = &changes.Files[i]
			break
		}
	}
	if file == nil {
		return result, fmt.Errorf("该文件已不在未提交列表中，请刷新")
	}
	if view == "staged" && !file.Staged {
		return result, fmt.Errorf("该文件没有已暂存差异")
	}
	if view == "working" && file.Untracked {
		result.Untracked = true
		filename := filepath.Join(changes.Root, filepath.FromSlash(path))
		info, err := os.Lstat(filename)
		if err != nil {
			return result, fmt.Errorf("无法读取未跟踪文件，请刷新")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result.Text, err = os.Readlink(filename)
			return result, err
		}
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf("该路径不是普通文本文件")
		}
		resolved, err := filepath.EvalSymlinks(filename)
		root, rootErr := filepath.EvalSymlinks(changes.Root)
		if err != nil || rootErr != nil {
			return result, fmt.Errorf("无法核验文件位置")
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return result, fmt.Errorf("文件链接指向工作树外，无法预览内容")
		}
		f, err := os.Open(filename)
		if err != nil {
			return result, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
		if err != nil {
			return result, err
		}
		result.Truncated = len(data) > 1024*1024
		if result.Truncated {
			data = data[:1024*1024]
		}
		result.Binary = bytes.ContainsRune(data, 0) || (!result.Truncated && !utf8.Valid(data))
		if !result.Binary {
			result.Text = strings.ToValidUTF8(string(data), "�")
		}
		return result, nil
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=none"}
	if view == "staged" {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	if file.OldPath != "" {
		args = append(args, file.OldPath)
	}
	data, truncated, err := readGit(changes.Root, 1024*1024, args...)
	if err != nil {
		return result, err
	}
	result.Text, result.Truncated = strings.ToValidUTF8(string(data), "�"), truncated
	result.Binary = bytes.Contains(data, []byte("Binary files ")) || bytes.Contains(data, []byte("GIT binary patch"))
	return result, nil
}
