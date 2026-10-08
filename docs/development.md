# 构建与发布

[← 返回首页](../README.md) · [使用指南](user-guide.md)

## 本地构建

在 Windows 环境中准备 Go 1.25+、Node.js / npm，以及 `go.mod` 对应的 Wails CLI。发布版用户无需安装这些开发工具。

```powershell
git clone https://github.com/ikMing0/project-runner.git
cd project-runner

go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
$env:Path += ';' + (Join-Path (go env GOPATH) 'bin')

npm.cmd --prefix frontend ci
npm.cmd --prefix frontend test
npm.cmd --prefix frontend run build
go test ./... -count=1 -timeout=5m
go vet ./...

wails build -o ProjectRunner.exe
```

产物位于 `build\bin\ProjectRunner.exe`。先生成前端资源，再运行 Go 测试，确保 `go:embed` 的目录存在。普通源码构建显示为「开发版」；正式版本号和源码提交由发布流程写入。

## GitHub Release

仓库提供 [Windows Release 工作流](../.github/workflows/release.yml)。将代码提交并推送后，创建并推送新的版本标签，即可自动测试、打包并发布。

以下 `v0.2.0` 是示例，请替换为准备发布且尚未使用的版本号：

```powershell
git tag -a v0.2.0 -m "ProjectRunner v0.2.0"
git push origin v0.2.0
```

请确认标签指向已推送、准备发布的提交。普通分支 push 不发布 Release；`v0.2.0-beta.1` 等带后缀的标签会标为预发布，不设为 Latest。已有同名 Release 不会被覆盖。

### 发布流程

| 步骤 | 内容 |
| --- | --- |
| 准备环境 | Windows x64、`go.mod` 对应的 Go / Wails、Node.js 22 |
| 构建前端 | `npm ci` 安装锁定依赖，生成 Wails 绑定，执行前端测试与构建 |
| 验证后端 | Go 测试与 `go vet` |
| 打包 | 写入版本号和源码提交，生成 Windows 可执行文件与 SHA256 |
| 发布 | 再次验证 SHA256，创建 GitHub Release 并生成更新记录 |

Release 附件包括：

- **`ProjectRunner.exe`**：Windows x64 可执行文件。
- **`SHA256SUMS.txt`**：下载文件的 SHA256 校验值。

下载程序仍需要 Windows 10 / 11 x64 和 WebView2；内置终端需要 Windows 10 1809 或更高版本。启动业务项目时，需准备其 JDK、Node.js、构建工具及数据库 / Redis 等依赖。发布包不包含本机项目配置、日志、终端会话或业务项目文件。

### 手动验证 Actions 构建

在 GitHub Actions 选择 **Windows Release → Run workflow**。手动运行仅保存构建附件，不创建 Release，附件保留 14 天。

发布使用 GitHub Actions 自带的 `GITHUB_TOKEN`，无需上传本机 SSH 私钥或配置个人 Token。仓库需要启用 Actions，并允许本工作流使用的 GitHub 官方 Actions 和发布任务的 `contents: write` 权限。

## 本机复现发布构建

使用 PowerShell 7，在仓库根目录执行相同的测试与打包流程：

```powershell
pwsh -File scripts/build-release.ps1 -Version v0.2.0
```

这个命令只在本机打包，不上传或发布。版本号同样为示例，产物位于 `build\bin\release\`。Wails 不在 PATH 时，可用 `-WailsExecutable` 指定 CLI 文件路径。
