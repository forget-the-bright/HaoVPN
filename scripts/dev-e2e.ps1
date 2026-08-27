#Requires -Version 7.0
<#
.SYNOPSIS
  本机 E2E 冒烟：构建 → 启动服务端 → 健康检查 →（可选 sudo 测 TUN）

.EXAMPLE
  .\scripts\dev-e2e.ps1
  .\scripts\dev-e2e.ps1 -WithSudo   # 用 sudo 启动以验证 TUN
#>
param(
    [switch]$WithSudo
)
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

Write-Host "==> 构建（含 wintun.dll）"
& .\scripts\build-local.ps1

$TestDir = Join-Path $Root "e2e-tmp"
if (Test-Path $TestDir) { Remove-Item -Recurse -Force $TestDir }
New-Item -ItemType Directory -Path $TestDir -Force | Out-Null
$TestDirUnix = ($TestDir -replace '\\', '/')

$ServerYaml = Join-Path $TestDir "server.yaml"
@"
# E2E 测试配置
server:
  listen: "127.0.0.1:18443"
  tls:
    cert_file: "$TestDirUnix/certs/server.crt"
    key_file: "$TestDirUnix/certs/server.key"
    auto_generate: true
vpn:
  subnet: "10.88.0.0/24"
  gateway_ip: "10.88.0.1"
  mtu: 1420
  heartbeat_timeout_sec: 30
  require_tun: false
nat:
  enabled: false
  allowed_lan_cidrs: []
database:
  path: "$TestDirUnix/data/haovpn.db"
api:
  listen_hosts: ["127.0.0.1"]
  port: 18080
  allow_public_bind: false
security:
  enforce_split_tunnel: true
admin:
  username: "admin"
  password: "changeme123"
log:
  level: "info"
  file: "$TestDirUnix/logs/server.log"
"@ | Set-Content -Path $ServerYaml -Encoding UTF8

$ServerExe = Join-Path $Root "bin\haovpn-server.exe"
$BinDir = Join-Path $Root "bin"

if ($WithSudo) {
    Write-Host "==> 启动服务端 (sudo)"
    $proc = Start-Process -FilePath "sudo" -ArgumentList @($ServerExe, "-c", $ServerYaml) -WorkingDirectory $BinDir -PassThru -WindowStyle Hidden
} else {
    Write-Host "==> 启动服务端（API 冒烟，TUN 可能 WARN）"
    $proc = Start-Process -FilePath $ServerExe -ArgumentList @("-c", $ServerYaml) -WorkingDirectory $BinDir -PassThru -WindowStyle Hidden
}

$ok = $false
for ($i = 0; $i -lt 15; $i++) {
    Start-Sleep -Seconds 1
    try {
        $resp = Invoke-RestMethod -Uri "http://127.0.0.1:18080/api/v1/health" -TimeoutSec 2
        if ($resp.db_ok) {
            Write-Host "==> 健康检查 OK: uptime=$($resp.uptime_sec)s online=$($resp.online_peers) db_ok=$($resp.db_ok)"
            $ok = $true
            break
        }
    } catch { }
}

$logPath = Join-Path $TestDir "logs/server.log"
if (Test-Path $logPath) {
    $tunLine = Get-Content $logPath | Select-String "TUN"
    if ($tunLine) { Write-Host "    TUN: $tunLine" }
}

if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
Remove-Item -Recurse -Force $TestDir -ErrorAction SilentlyContinue

if (-not $ok) { throw "E2E 健康检查失败" }
Write-Host "==> E2E 冒烟通过"
