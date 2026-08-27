#Requires -Version 7.0
<#
.SYNOPSIS
  全平台交叉编译 release 包 → dist/

.DESCRIPTION
  读取根目录 VERSION（仅开发者维护）与 scripts/platforms.txt。
  默认构建 6 个平台 × server + client = 12 个二进制。

.PARAMETER Platform
  仅构建指定平台，可多次指定。例: -Platform linux/amd64 -Platform windows/arm64

.PARAMETER ServerOnly
  只构建 server

.PARAMETER ClientOnly
  只构建 client

.PARAMETER NoZip
  不生成各平台 zip 包

.EXAMPLE
  .\scripts\build-release.ps1
  .\scripts\build-release.ps1 -Platform windows/amd64
  .\scripts\build-release.ps1 -ServerOnly
#>
[CmdletBinding()]
param(
    [string[]]$Platform = @(),
    [switch]$ServerOnly,
    [switch]$ClientOnly,
    [switch]$NoZip
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/lib/build-common.ps1"

$Root = Get-ProjectRoot
Set-Location $Root

if (-not (Test-Path "go.mod")) {
    Write-Error "未找到 go.mod"
}

# 确保 Windows 本地有 wintun.dll 供 release 复制
& "$PSScriptRoot/install-wintun.ps1"

$Version = Get-ProjectVersion -Root $Root
$Commit = Get-GitCommitShort -Root $Root
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = Get-BuildLdflags -Version $Version -Commit $Commit -BuildTime $BuildTime
$Out = "dist"

$allPlatforms = Get-PlatformsFromFile -Root $Root
if ($Platform.Count -gt 0) {
    $selected = @()
    foreach ($p in $Platform) {
        $parts = $p -split '/'
        if ($parts.Count -ne 2) { throw "无效平台格式: $p（应为 goos/goarch）" }
        $selected += [PSCustomObject]@{ GOOS = $parts[0]; GOARCH = $parts[1] }
    }
    $platforms = $selected
} else {
    $platforms = $allPlatforms
}

$buildServer = -not $ClientOnly
$buildClient = -not $ServerOnly
if (-not $buildServer -and -not $buildClient) {
    $buildServer = $true
    $buildClient = $true
}

Write-Host "==> HaoVPN release build"
Write-Host "    version: $Version"
Write-Host "    commit:  $Commit"
Write-Host "    time:    $BuildTime"
Write-Host "    targets: $($platforms.Count) platform(s)"
Write-Host ""

if (Test-Path $Out) { Remove-Item -Recurse -Force $Out }
New-Item -ItemType Directory -Path $Out -Force | Out-Null
Copy-Item (Join-Path $Root "VERSION") (Join-Path $Out "VERSION")

$artifacts = [System.Collections.Generic.List[object]]::new()

try {
    foreach ($plat in $platforms) {
        $goos = $plat.GOOS
        $goarch = $plat.GOARCH
        $dir = Join-Path $Out "$goos-$goarch"
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $ext = if ($goos -eq "windows") { ".exe" } else { "" }

        if ($buildServer) {
            $outPath = Join-Path $dir "haovpn-server$ext"
            Write-Host "==> server  $goos/$goarch"
            Invoke-GoBuild -Root $Root -Goos $goos -Goarch $goarch `
                -Package "./cmd/server" -OutputPath $outPath -Ldflags $Ldflags
            $artifacts.Add([ordered]@{ type = "server"; platform = "$goos/$goarch"; path = $outPath })
        }

        if ($buildClient) {
            $outPath = Join-Path $dir "haovpn-client$ext"
            Write-Host "==> client  $goos/$goarch"
            Invoke-GoBuild -Root $Root -Goos $goos -Goarch $goarch `
                -Package "./cmd/client" -OutputPath $outPath -Ldflags $Ldflags
            $artifacts.Add([ordered]@{ type = "client"; platform = "$goos/$goarch"; path = $outPath })
        }

        # Windows 产物附带 wintun.dll
        if ($goos -eq "windows") {
            $wintunSrc = Join-Path $Root "bin/wintun.dll"
            if (Test-Path $wintunSrc) {
                Copy-Item $wintunSrc (Join-Path $dir "wintun.dll") -Force
                Write-Host "    + wintun.dll"
            } else {
                Write-Warning "bin/wintun.dll 不存在，请先运行 build-local 或 install-wintun.ps1"
            }
        }

        if (-not $NoZip) {
            $zipName = "HaoVPN-$Version-$goos-$goarch.zip"
            $zipPath = Join-Path $Out $zipName
            if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
            Compress-Archive -Path "$dir/*" -DestinationPath $zipPath -CompressionLevel Optimal
            Write-Host "    zip: $zipName"
        }
    }
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-ReleaseManifest -OutDir $Out -Root $Root -Version $Version -Commit $Commit -BuildTime $BuildTime -Artifacts $artifacts

Write-Host ""
Write-Host "完成。产物目录: $Out/"
Write-Host "平台列表:"
Get-Content (Join-Path $Root "scripts/platforms.txt") | Where-Object { $_ -and -not $_.StartsWith("#") } | ForEach-Object { Write-Host "  - $_" }
Get-ChildItem -Recurse $Out -File | Sort-Object FullName | ForEach-Object { Write-Host "  $($_.FullName)" }
