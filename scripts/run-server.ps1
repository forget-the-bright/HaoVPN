#Requires -Version 7.0
<#
.SYNOPSIS
  启动 HaoVPN 服务端：已是管理员则直接运行，否则通过 sudo 提权。
.EXAMPLE
  .\scripts\run-server.ps1
  .\scripts\run-server.ps1 -Config .\home\server.yaml
#>
param(
    [string]$Config = ".\home\server.yaml"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$exe = Join-Path $Root "bin\haovpn-server.exe"
if (-not (Test-Path $exe)) {
    Write-Host "未找到 $exe，请先运行 .\scripts\build-local.ps1" -ForegroundColor Red
    exit 1
}

$cfgPath = if ([System.IO.Path]::IsPathRooted($Config)) { $Config } else { Join-Path $Root $Config }
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if ($isAdmin) {
    Write-Host "管理员模式，直接启动服务端…" -ForegroundColor Green
    & $exe -c $cfgPath
} else {
    Write-Host "非管理员，使用 sudo 提权…" -ForegroundColor Yellow
    $sudo = Join-Path $env:SystemRoot "System32\sudo.exe"
    if (-not (Test-Path $sudo)) { $sudo = "sudo" }
    & $sudo $exe -c $cfgPath
}
exit $LASTEXITCODE
