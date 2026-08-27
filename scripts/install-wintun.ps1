#Requires -Version 7.0
<#
.SYNOPSIS
  下载 Wintun 0.14.1 到 internal/tun/wintundll/（go:embed 进 Windows 客户端，单 exe 分发）。

.PARAMETER Arch
  amd64 | arm64 | all（默认 all，供交叉编译 embed）

.EXAMPLE
  .\scripts\install-wintun.ps1
  .\scripts\install-wintun.ps1 -Arch amd64
#>
param(
    [ValidateSet("amd64", "arm64", "all")]
    [string]$Arch = "all"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$EmbedRoot = Join-Path $Root "internal\tun\wintundll"
$WintunVer = "0.14.1"
$ZipUrl = "https://www.wintun.net/builds/wintun-$WintunVer.zip"

$targets = @()
if ($Arch -eq "all") { $targets = @("amd64", "arm64") } else { $targets = @($Arch) }

function Test-EmbedReady {
    param([string[]]$ArchList)
    foreach ($a in $ArchList) {
        $p = Join-Path $EmbedRoot "$a\wintun.dll"
        if (-not (Test-Path $p)) { return $false }
    }
    return $true
}

if (Test-EmbedReady -ArchList $targets) {
    Write-Host "wintun embed 已就绪: $EmbedRoot ($($targets -join ', '))"
    exit 0
}

$Zip = Join-Path $env:TEMP "wintun-$WintunVer-$([Guid]::NewGuid().ToString('n').Substring(0,8)).zip"
$Extract = Join-Path $env:TEMP "wintun-extract-$([Guid]::NewGuid().ToString('n').Substring(0,8))"

Write-Host "==> 下载 Wintun $WintunVer"
Invoke-WebRequest -Uri $ZipUrl -OutFile $Zip
New-Item -ItemType Directory -Path $Extract -Force | Out-Null
& tar -xf $Zip -C $Extract

foreach ($a in $targets) {
    $Src = Join-Path $Extract "wintun\bin\$a\wintun.dll"
    if (-not (Test-Path $Src)) {
        throw "未找到 wintun/bin/$a/wintun.dll"
    }
    $DestDir = Join-Path $EmbedRoot $a
    New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
    Copy-Item $Src (Join-Path $DestDir "wintun.dll") -Force
    Write-Host "    embed -> internal/tun/wintundll/$a/wintun.dll"
}

Remove-Item $Zip -Force -ErrorAction SilentlyContinue
Remove-Item $Extract -Recurse -Force -ErrorAction SilentlyContinue
Write-Host "==> Wintun embed 完成（构建时打入 exe，运行时释放到 exe 同目录）"
