#Requires -Version 7.0
<#
.SYNOPSIS
  本机快速构建（当前 Windows 环境，默认 amd64）

.EXAMPLE
  .\scripts\build-local.ps1
  .\scripts\build-local.ps1 -Arch arm64
#>
[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [switch]$ServerOnly,
    [switch]$ClientOnly
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/lib/build-common.ps1"

$Root = Get-ProjectRoot
Set-Location $Root

if (-not (Test-Path "go.mod")) {
    Write-Error "未找到 go.mod，请先初始化 Go 模块"
}

$Version = Get-ProjectVersion -Root $Root
$Commit = Get-GitCommitShort -Root $Root
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = Get-BuildLdflags -Version $Version -Commit $Commit -BuildTime $BuildTime

# Fyne 元数据 Version 与根目录 VERSION 对齐（勿在 FyneApp.toml 手写死版本）
Sync-FyneAppTomlFromVersion -Root $Root -Version $Version

$Out = Join-Path $Root "bin"
New-Item -ItemType Directory -Path $Out -Force | Out-Null

$env:GOOS = "windows"
$env:GOARCH = $Arch
$env:CGO_ENABLED = "0"

Write-Host "==> local build windows/$Arch  version=$Version"

# embed 依赖：构建前须有 wintun.dll（单 exe 分发，运行时释放）
& "$PSScriptRoot/install-wintun.ps1" -Arch $Arch

try {
    if (-not $ClientOnly) {
        Invoke-GoBuild -Root $Root -Goos "windows" -Goarch $Arch `
            -Package "./cmd/server" -OutputPath (Join-Path $Out "haovpn-server.exe") -Ldflags $Ldflags
        Write-Host "    server -> bin/haovpn-server.exe"
    }
    if (-not $ServerOnly) {
        Invoke-GoBuild -Root $Root -Goos "windows" -Goarch $Arch `
            -Package "./cmd/client" -OutputPath (Join-Path $Out "haovpn-client.exe") -Ldflags $Ldflags
        Write-Host "    client -> bin/haovpn-client.exe"
        Invoke-GoBuildGui -Root $Root -Goarch $Arch `
            -OutputPath (Join-Path $Out "haovpn-client-gui.exe") -Ldflags $Ldflags
        Write-Host "    client-gui -> bin/haovpn-client-gui.exe"
    }
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host "完成。"
