#Requires -Version 7.0
<#
.SYNOPSIS
  连续启动/停止服务端，检查 Wintun 适配器日志与网卡孤儿名（需管理员）。

.EXAMPLE
  .\scripts\test-wintun-restart.ps1 -ConfigPath .\home\server.yaml
#>
param(
  [string]$ConfigPath = ".\home\server.yaml",
  [int]$Cycles = 3,
  [string]$LiveLog = ".\home\logs\server.live.log"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$exe = Join-Path $Root "bin\haovpn-server.exe"
if (-not (Test-Path $exe)) {
  Write-Host "请先 build-local: $exe 不存在" -ForegroundColor Red
  exit 1
}

function Get-WintunAdapters {
  Get-NetAdapter -ErrorAction SilentlyContinue |
    Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } |
    Select-Object Name, InterfaceDescription, Status
}

Write-Host "==> Wintun 重启实测 ($Cycles 轮) ==" -ForegroundColor Cyan
$before = Get-WintunAdapters
Write-Host "启动前 Wintun 网卡:" ($before | Format-Table | Out-String)

$fail = 0
for ($i = 1; $i -le $Cycles; $i++) {
  Write-Host "`n--- 第 $i 轮 ---" -ForegroundColor Yellow
  $p = Start-Process -FilePath $exe -ArgumentList @("-c", $ConfigPath) -PassThru -WindowStyle Hidden
  Start-Sleep -Seconds 8
  if ($p.HasExited) {
    Write-Host "服务端异常退出 exit=$($p.ExitCode)" -ForegroundColor Red
    $fail++
    break
  }
  Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 3
}

if (Test-Path $LiveLog) {
  $tail = Get-Content $LiveLog -Tail 80 -ErrorAction SilentlyContinue
  $noise = $tail | Where-Object { $_ -match 'Failed to find matching adapter' -and $_ -notmatch '\[DEBUG\]' }
  if ($noise) {
    Write-Host "WARN: live.log 仍有未降为 DEBUG 的 matching adapter 行:" -ForegroundColor Yellow
    $noise | ForEach-Object { Write-Host "  $_" }
    $fail++
  }
  $reuse = $tail | Where-Object { $_ -match '已复用|清理后复用|OpenAdapter 成功' }
  if ($reuse) {
    Write-Host "OK: 检测到适配器复用相关日志" -ForegroundColor Green
  }
} else {
  Write-Host "WARN: 未找到 $LiveLog" -ForegroundColor Yellow
}

$after = Get-WintunAdapters
$orphans = $after | Where-Object { $_.Name -match 'haovpn0\s+\d' }
if ($orphans) {
  Write-Host "FAIL: 仍存在孤儿网卡名:" -ForegroundColor Red
  $orphans | Format-Table
  $fail++
} else {
  Write-Host "OK: 无 haovpn0 N 类孤儿网卡名" -ForegroundColor Green
}

if ($fail -gt 0) { exit 1 }
Write-Host "`n全部检查通过" -ForegroundColor Green
