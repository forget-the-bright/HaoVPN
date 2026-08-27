#Requires -Version 7.0
<#
.SYNOPSIS
  下载 wintun.dll 到 bin/（Windows TUN 运行依赖，与 exe 同目录）

.EXAMPLE
  .\scripts\install-wintun.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$OutDir = Join-Path $Root "bin"
$Dll = Join-Path $OutDir "wintun.dll"

if (Test-Path $Dll) {
    Write-Host "wintun.dll 已存在: $Dll"
    exit 0
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$Zip = Join-Path $env:TEMP "wintun-$([Guid]::NewGuid().ToString('n').Substring(0,8)).zip"
$Extract = Join-Path $env:TEMP "wintun-extract-$([Guid]::NewGuid().ToString('n').Substring(0,8))"

Write-Host "==> 下载 Wintun 0.14.1"
Invoke-WebRequest -Uri "https://www.wintun.net/builds/wintun-0.14.1.zip" -OutFile $Zip

New-Item -ItemType Directory -Path $Extract -Force | Out-Null
# Expand-Archive 可能漏文件，用 tar 解压
& tar -xf $Zip -C $Extract

$Src = Join-Path $Extract "wintun/bin/amd64/wintun.dll"
if (-not (Test-Path $Src)) {
    throw "未找到 wintun/bin/amd64/wintun.dll"
}
Copy-Item $Src $Dll -Force
Remove-Item $Zip -Force -ErrorAction SilentlyContinue
Remove-Item $Extract -Recurse -Force -ErrorAction SilentlyContinue
Write-Host "==> 已安装 $Dll"
