#Requires -Version 7.0
<#
.SYNOPSIS
  打包 ZeroTier 跨网现场验收目录与 zip（含最新本机二进制）。

.EXAMPLE
  .\scripts\pack-zt-field-test.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

& "$PSScriptRoot/build-local.ps1" | Out-Null

$OutDir = Join-Path $Root "dist\zt-field-test"
$BinDir = Join-Path $OutDir "bin"
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

Copy-Item -Force (Join-Path $Root "bin\haovpn-server.exe") $BinDir
Copy-Item -Force (Join-Path $Root "bin\haovpn-client.exe") $BinDir

# 同步 home 配置模板（不覆盖现场已改数据库）
$homeTpl = Join-Path $OutDir "home"
New-Item -ItemType Directory -Path $homeTpl -Force | Out-Null
Copy-Item -Force (Join-Path $Root "home\server.yaml") $homeTpl

$Zip = Join-Path $Root "dist\HaoVPN-zt-field-test.zip"
if (Test-Path $Zip) { Remove-Item -Force $Zip }
Compress-Archive -Path (Join-Path $Root "dist\zt-field-test\*") -DestinationPath $Zip -Force

Write-Host "==> 现场包已更新:"
Write-Host "    目录: $OutDir"
Write-Host "    ZIP:  $Zip"
$ver = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim()
Write-Host "    版本: $ver"
