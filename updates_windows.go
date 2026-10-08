//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Release builds set these through -ldflags. Local builds identify as dev.
var buildVersion = "dev"
var buildCommit = "unknown"

const releaseRepository = "ikMing0/project-runner"
const releasesURL = "https://github.com/" + releaseRepository + "/releases"

type BuildInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	ReleasesURL string `json:"releasesUrl"`
}
type ReleaseCheck struct {
	Current       BuildInfo `json:"current"`
	LatestVersion string    `json:"latestVersion"`
	ReleaseURL    string    `json:"releaseUrl"`
	PublishedAt   string    `json:"publishedAt"`
	Comparable    bool      `json:"comparable"`
	HasUpdate     bool      `json:"hasUpdate"`
	Message       string    `json:"message"`
	CheckedAt     int64     `json:"checkedAt"`
}
type latestRelease struct {
	Tag         string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

var releaseTagPattern = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

func (a *App) GetBuildInfo() BuildInfo { return BuildInfo{buildVersion, buildCommit, releasesURL} }

func releaseVersion(tag string) ([3]uint64, bool, bool) {
	var version [3]uint64
	match := releaseTagPattern.FindStringSubmatch(tag)
	if match == nil {
		return version, false, false
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return version, false, false
		}
		version[i] = n
	}
	return version, match[4] != "", true
}
func newerRelease(current, latest string) (bool, bool) {
	old, oldPre, oldOK := releaseVersion(current)
	next, nextPre, nextOK := releaseVersion(latest)
	if !oldOK || !nextOK || nextPre {
		return false, false
	}
	for i := 0; i < 3; i++ {
		if next[i] != old[i] {
			return next[i] > old[i], true
		}
	}
	return oldPre, true
}
func makeReleaseCheck(info BuildInfo, data []byte) (ReleaseCheck, error) {
	result := ReleaseCheck{Current: info, ReleaseURL: releasesURL, CheckedAt: time.Now().UnixMilli()}
	var release latestRelease
	if len(data) > 512*1024 || json.Unmarshal(data, &release) != nil || release.Draft || release.Prerelease {
		return result, fmt.Errorf("无法读取正式 Release 信息")
	}
	if _, _, ok := releaseVersion(release.Tag); !ok {
		return result, fmt.Errorf("Release 版本号格式无效")
	}
	result.LatestVersion, result.PublishedAt = release.Tag, release.PublishedAt
	result.ReleaseURL = releasesURL + "/tag/" + url.PathEscape(release.Tag)
	result.HasUpdate, result.Comparable = newerRelease(info.Version, release.Tag)
	switch {
	case !result.Comparable:
		result.Message = "当前为开发版，正式发布版本为 " + release.Tag + "；请查看更新说明再决定是否下载。"
	case result.HasUpdate:
		result.Message = "发现新版本 " + release.Tag
	default:
		result.Message = "当前版本已不低于最新正式版本 " + release.Tag
	}
	return result, nil
}
func fetchLatestRelease(ctx context.Context) ([]byte, error) {
	if gh, err := exec.LookPath("gh.exe"); err == nil {
		cmd := exec.CommandContext(ctx, gh, "api", "--hostname", "github.com", "--method", "GET", "repos/"+releaseRepository+"/releases/latest")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		var output versionOutput
		// versionOutput caps at 16 KiB; request selected fields to avoid bodies/assets.
		cmd.Args = append(cmd.Args, "--jq", "{tag_name: .tag_name, published_at: .published_at, draft: .draft, prerelease: .prerelease}")
		cmd.Stdout = &output
		cmd.WaitDelay = time.Second
		if err = cmd.Run(); err == nil {
			return output.Bytes(), nil
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("检查更新超时，请稍后重试")
		}
		// No credential files or tokens are read by this app.
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+releaseRepository+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "ProjectRunner")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接 GitHub，请稍后重试或打开发布页面")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return nil, fmt.Errorf("私有仓库需要本机 GitHub CLI 登录，或在浏览器打开发布页面；也可能尚无正式 Release")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 %d，请稍后重试或打开发布页面", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 512*1024+1))
	if err != nil {
		return nil, fmt.Errorf("读取更新信息失败")
	}
	return data, nil
}
func (a *App) CheckForUpdates() (ReleaseCheck, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	data, err := fetchLatestRelease(ctx)
	if err != nil {
		return ReleaseCheck{Current: a.GetBuildInfo(), ReleaseURL: releasesURL, Message: strings.TrimSpace(err.Error())}, err
	}
	return makeReleaseCheck(a.GetBuildInfo(), data)
}
