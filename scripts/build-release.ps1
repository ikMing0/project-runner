#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$')]
    [string]$Version,
    [string]$WailsExecutable = 'wails'
)

$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'Windows release builds must run on Windows.' }

function Invoke-Checked {
    param([string]$Executable, [string[]]$Arguments)
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Executable failed with exit code $LASTEXITCODE." }
}

$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    # The Go embed directory must exist before Wails compiles its bindings generator.
    New-Item -ItemType Directory -Path 'frontend/dist' -Force | Out-Null
    New-Item -ItemType File -Path 'frontend/dist/.gitkeep' -Force | Out-Null
    Invoke-Checked $WailsExecutable @('generate', 'module')

    Push-Location 'frontend'
    try {
        Invoke-Checked 'npm.cmd' @('ci')
        Invoke-Checked 'npm.cmd' @('test')
        Invoke-Checked 'npm.cmd' @('run', 'build')
    } finally { Pop-Location }

    Invoke-Checked 'go' @('test', './...', '-count=1', '-timeout=5m')
    Invoke-Checked 'go' @('vet', './...')
    # Frontend assets and bindings were generated above from this checkout.
    Invoke-Checked $WailsExecutable @('build', '-s', '-skipbindings', '-m', '-nosyncgomod', '-trimpath', '-platform', 'windows/amd64', '-o', 'ProjectRunner.exe')

    $releaseDirectory = Join-Path $projectRoot 'build/bin/release'
    New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
    $releaseExecutable = Join-Path $releaseDirectory 'ProjectRunner.exe'
    Copy-Item -LiteralPath 'build/bin/ProjectRunner.exe' -Destination $releaseExecutable -Force
    $checksum = (Get-FileHash -LiteralPath $releaseExecutable -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $releaseDirectory 'SHA256SUMS.txt') -Value "$checksum  ProjectRunner.exe`n" -Encoding utf8NoBOM -NoNewline
    $commit = & git rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Cannot identify the source commit.' }
    $notes = @"
Windows x64 版本：$Version
源码提交：$commit

下载 ProjectRunner.exe 后直接运行，无需安装 Go、Wails 或开发依赖。
系统需要 Windows 10/11 x64 和 Microsoft Edge WebView2 Runtime；内置终端需要 Windows 10 1809 或更高版本。
启动业务项目时，仍需配置该项目使用的 JDK、Node.js 和 Maven/Gradle 等工具。
SHA256SUMS.txt 用于核验下载文件的 SHA256。
"@
    Set-Content -LiteralPath (Join-Path $releaseDirectory 'release-notes.md') -Value $notes -Encoding utf8NoBOM
    Write-Host "Release assets: $releaseDirectory"
} finally { Pop-Location }
