#Requires -Version 7.0
<#
.SYNOPSIS
  v1.0 现场交付硬门禁（TUN+NAT 真路径 + 客户端在线 + PLC + Windows 服务）

.DESCRIPTION
  smoke（dev-acceptance.ps1）通过 ≠ v1.0 可交付。
  本脚本 0 FAIL、0 SKIP 才允许在文档中标记 step11 完成。

.EXAMPLE
  .\scripts\dev-field-gate.ps1 -PlcHost 192.168.1.10
  .\scripts\dev-field-gate.ps1 -PlcHost 192.168.1.10 -LanCidr "192.168.1.0/24" -UseHomeConfig
  .\scripts\dev-field-gate.ps1 -PlcHost 192.168.1.10 -SkipSmoke
#>
param(
    [Parameter(Mandatory = $true)]
    [string]$PlcHost,
    [string]$LanCidr = "192.168.1.0/24",
    [switch]$UseHomeConfig,
    [switch]$SkipSmoke
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root
. (Join-Path $PSScriptRoot "lib/field-common.ps1")

$FieldPass = 0
$FieldFail = 0
$FieldResults = [System.Collections.Generic.List[string]]::new()

Write-Host "========================================"
Write-Host "  HaoVPN v1.0 现场交付硬门禁 (field gate)"
Write-Host "  PlcHost=$PlcHost  LanCidr=$LanCidr"
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "  警告: 当前 shell 非管理员；TUN/NAT/服务依赖 sudo 提权（需 UAC 批准）" -ForegroundColor Yellow
}
Write-Host "========================================"

# --- 0. 硬断言单测 ---
Write-Host "`n==> [0] P0/P1 硬断言 go test"
$hardTests = @(
    "TestPeerPrivateKeyAESAndExport",
    "TestLogsAPIContainsMarker",
    "TestMigratePlaintextPeerKeys",
    "TestProbeMTUEnqueued",
    "TestNewStatusIncludesRecentErrors",
    "TestHandshakeReconnectNoDeadlock",
    "TestReplayRejected",
    "TestLateralVPNIPBlocked"
)
$run = ($hardTests | ForEach-Object { [regex]::Escape($_) }) -join "|"
go test -count=1 -run $run ./internal/api/... ./internal/persist/... ./internal/transport/... ./internal/health/... ./internal/tunnel/... ./internal/crypto/... ./internal/sessionmgr/...
New-FieldRecord "硬断言 go test" ($LASTEXITCODE -eq 0)

# --- 1. 构建 ---
Write-Host "`n==> [1] 本机构建"
& "$PSScriptRoot/build-local.ps1" | Out-Null
New-FieldRecord "build-local" (Test-Path (Join-Path $Root "bin\haovpn-server.exe"))

# --- 2. 可选 smoke ---
if (-not $SkipSmoke) {
    Write-Host "`n==> [2] smoke（无管理员，require_tun=false）"
    & "$PSScriptRoot/dev-acceptance.ps1"
    New-FieldRecord "smoke dev-acceptance" ($LASTEXITCODE -eq 0)
} else {
    Write-Host "`n==> [2] smoke 已跳过 (-SkipSmoke)"
}

# --- 3. 现场 dataplane（require_tun + NAT）---
Write-Host "`n==> [3] 现场 dataplane（sudo，对齐 home/server.yaml）"
$FieldDir = Join-Path $Root "field-tmp"
if (Test-Path $FieldDir) { Remove-Item -Recurse -Force $FieldDir }
New-Item -ItemType Directory -Path $FieldDir -Force | Out-Null

$apiPort = 19090
$tunnelPort = 19450
$fieldYaml = Join-Path $FieldDir "server.yaml"
$serverLog = Join-Path $FieldDir "logs\server.log"
New-FieldServerYaml $FieldDir $apiPort $tunnelPort $LanCidr | Set-Content $fieldYaml -Encoding UTF8

$health = $null
$fieldProc = Start-FieldServer $fieldYaml $Root
$serverProc = Wait-FieldServerProcess -Sec 20
if (-not $serverProc) {
    $earlyLog = Read-LiveLogText $serverLog
    $hint = if ($earlyLog -match "TUN 创建失败") { "TUN 无管理员" } else { "sudo/UAC 未批准或服务未启动" }
    New-FieldRecord "field haovpn-server 进程" $false $hint
    New-FieldRecord "field 服务端进程存活" (-not $fieldProc.HasExited) "wrapper exit=$($fieldProc.ExitCode)"
    New-FieldRecord "live.log TUN IP 已配置" $false $hint
    New-FieldRecord "live.log NAT 非空话" $false $hint
    New-FieldRecord "live.log 无 TUN 创建失败" (($earlyLog -notmatch "TUN 创建失败") -and ($earlyLog.Length -gt 0))
} else {
    New-FieldRecord "field haovpn-server 进程" $true "pid=$($serverProc.Id)"
    New-FieldRecord "field 服务端进程存活" $true
    $health = Wait-FieldHealth $apiPort 40 -RequireTunNat
    $healthDetail = if ($health) {
        "tun=$($health.tun_ok) nat=$($health.nat_ok)"
    } elseif ($script:FieldHealthLast.error) {
        "无响应: $($script:FieldHealthLast.error)"
    } elseif ($script:FieldHealthLast) {
        "tun=$($script:FieldHealthLast.tun_ok) nat=$($script:FieldHealthLast.nat_ok) db=$($script:FieldHealthLast.db_ok)"
    } else {
        "health 超时"
    }
    New-FieldRecord "health tun_ok+nat_ok" ($null -ne $health) $healthDetail

    $liveText = Read-LiveLogText $serverLog
    $liveCheck = Assert-FieldServerLiveLog $liveText
    New-FieldRecord "live.log TUN IP 已配置" $liveCheck.Tun
    New-FieldRecord "live.log NAT 非空话" ($liveCheck.Nat -and $liveCheck.NoFake)
    New-FieldRecord "live.log 无 TUN 创建失败" $liveCheck.NoTunFail
}

$fieldDataplaneOk = ($null -ne $health) -and ($null -ne $serverProc)
$clientProc = $null

if ($fieldDataplaneOk) {
$base = "http://127.0.0.1:$apiPort"
$auth = Invoke-FieldLogin $base
$ws = $auth.Session
$csrf = $auth.CSRF
$headers = @{ "X-CSRF-Token" = $csrf }

$pr = Invoke-WebRequest -Uri "$base/api/v1/users" -Method POST -Body @{
    username = "field_eng"
    password = "FieldTest123!"
    ip_mode  = "fixed"
} -Headers $headers -WebSession $ws -UseBasicParsing
$userId = ($pr.Content | ConvertFrom-Json).id
New-FieldRecord "field 创建 VPN 账号" ($userId -gt 0)

$zipPath = Join-Path $FieldDir "client.zip"
Invoke-WebRequest -Uri "$base/api/v1/users/$userId/export.zip" -WebSession $ws -OutFile $zipPath -UseBasicParsing
$clientDir = Join-Path $FieldDir "client-bundle"
$serverAddr = "127.0.0.1:$tunnelPort"
$clientYaml = Expand-FieldClientZip $zipPath $clientDir $serverAddr

# 复制到 bin（Windows 服务从 exe 同目录读 client.yaml）
Copy-Item $clientYaml (Join-Path $Root "bin\client.yaml") -Force

$clientProc = Start-FieldClient $clientYaml $Root
$onlineOk = $false
$dash = $null
for ($i = 0; $i -lt 12; $i++) {
    Start-Sleep -Seconds 1
    try {
        $dash = Invoke-RestMethod -Uri "$base/api/v1/dashboard" -WebSession $ws
        $online = $dash.online_accounts
        if ($null -eq $online) { $online = $dash.online_peers }
        if ($online -ge 1) { $onlineOk = $true; break }
    } catch { }
}
New-FieldRecord "client ≤12s Dashboard 在线" $onlineOk "online=$($dash.online_accounts)$($dash.online_peers)"

$clientLog = Join-Path $clientDir "logs\client.log"
$clientLive = Read-LiveLogText (Join-Path $Root "bin\logs\client.log")
if (-not $clientLive) { $clientLive = Read-LiveLogText $clientLog }
New-FieldRecord "client 握手成功日志" ($clientLive -match "隧道握手成功")
New-FieldRecord "client MTU 日志" (($clientLive -match "mtu=") -or ($clientLive -match "已应用服务端策略"))

# --- 4. PLC ping + 流量 ---
Write-Host "`n==> [4] PLC ping + 流量统计"
$monBefore = Invoke-RestMethod -Uri "$base/api/v1/monitor/online" -WebSession $ws
$itemBefore = $monBefore.items | Where-Object { $_.peer_id -eq $peerId } | Select-Object -First 1
$rx0 = [int64]($itemBefore.rx_bytes ?? 0)
$tx0 = [int64]($itemBefore.tx_bytes ?? 0)

$ping = ping -n 4 $PlcHost 2>&1 | Out-String
$pingOk = ($ping -match "TTL=") -or ($ping -match "ttl=") -or ($ping -match "来自") -or ($ping -match "Reply from")
New-FieldRecord "PLC ping $PlcHost" $pingOk $ping.Trim()

Start-Sleep -Seconds 2
$monAfter = Invoke-RestMethod -Uri "$base/api/v1/monitor/online" -WebSession $ws
$itemAfter = $monAfter.items | Where-Object { $_.peer_id -eq $peerId } | Select-Object -First 1
$rx1 = [int64]($itemAfter.rx_bytes ?? 0)
$tx1 = [int64]($itemAfter.tx_bytes ?? 0)
$trafficUp = ($rx1 -gt $rx0) -or ($tx1 -gt $tx0)
New-FieldRecord "monitor 流量上涨" $trafficUp "rx $rx0->$rx1 tx $tx0->$tx1"

# --- 5. Windows 服务烟测 ---
Write-Host "`n==> [5] Windows 服务 install/start/stop/uninstall"
Stop-FieldProc $clientProc
Start-Sleep -Seconds 2

$clientExe = Join-Path $Root "bin\haovpn-client.exe"
$svcInstall = & sudo $clientExe --service install 2>&1 | Out-String
$svcInstalled = ($svcInstall -match "服务已安装") -or ($svcInstall -match "服务已存在")
New-FieldRecord "service install" $svcInstalled $svcInstall.Trim()

if ($svcInstalled) {
    $svcStart = & sudo $clientExe --service start 2>&1 | Out-String
    New-FieldRecord "service start" ($svcStart -match "服务已启动") $svcStart.Trim()
    Start-Sleep -Seconds 10
    $dashSvc = Invoke-RestMethod -Uri "$base/api/v1/dashboard" -WebSession $ws
    New-FieldRecord "service 运行后在线" ($dashSvc.online_peers -ge 1) "online=$($dashSvc.online_peers)"

    $svcStop = & sudo $clientExe --service stop 2>&1 | Out-String
    New-FieldRecord "service stop" ($svcStop -match "服务已停止") $svcStop.Trim()
    Start-Sleep -Seconds 2

    $svcUn = & sudo $clientExe --service uninstall 2>&1 | Out-String
    New-FieldRecord "service uninstall" ($svcUn -match "服务已卸载") $svcUn.Trim()
}

} else {
    Write-Host "`n==> [3+] dataplane 未就绪，跳过 peer/PLC/服务烟测" -ForegroundColor Yellow
    New-FieldRecord "field 创建 peer" $false "dataplane 未就绪"
    New-FieldRecord "client ≤12s Dashboard 在线" $false "dataplane 未就绪"
    New-FieldRecord "client 握手成功日志" $false "dataplane 未就绪"
    New-FieldRecord "client MTU 日志" $false "dataplane 未就绪"
    New-FieldRecord "PLC ping $PlcHost" $false "dataplane 未就绪"
    New-FieldRecord "monitor 流量上涨" $false "dataplane 未就绪"
    New-FieldRecord "service install" $false "dataplane 未就绪"
}

# --- 6. home/server.yaml 实跑（可选）---
if ($UseHomeConfig) {
    Write-Host "`n==> [6] home/server.yaml live 验证"
    Stop-FieldProc $fieldProc
    Start-Sleep -Seconds 2

    $homeYaml = Join-Path $Root "home\server.yaml"
    $homeProc = Start-FieldServer $homeYaml $Root
    Start-Sleep -Seconds 8
    $homeLive = Read-LiveLogText (Join-Path $Root "home\logs\server.log")
    $homeCheck = Assert-FieldServerLiveLog $homeLive
    New-FieldRecord "home live TUN" $homeCheck.Tun
    New-FieldRecord "home live NAT" ($homeCheck.Nat -and $homeCheck.NoFake)
    Stop-FieldProc $homeProc
} else {
    Write-Host "`n==> [6] home 配置跳过（加 -UseHomeConfig 启用）"
}

Stop-FieldProc $fieldProc
Stop-FieldProc $clientProc
Get-Process haovpn-server,haovpn-client -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Remove-Item -Recurse -Force $FieldDir -ErrorAction SilentlyContinue

# --- 汇总 ---
Write-Host "`n========================================"
Write-Host "  field 门禁汇总: PASS=$FieldPass  FAIL=$FieldFail"
Write-Host "========================================"
foreach ($line in $FieldResults) {
    if ($line.StartsWith("[FAIL]")) { Write-Host $line -ForegroundColor Red }
}
Write-Host ""
Write-Host "【手工确认 — step11.12 重启自连】" -ForegroundColor Yellow
Write-Host "  1. 安装 client 服务并重启机器"
Write-Host "  2. 确认 Dashboard 在线、VPN IP 不变"
Write-Host "  3. 在 docs/dev-log.md 写入「step11.12 reboot OK」及日期"
Write-Host "  未勾选前文档不得写「服务开机自连已验收」"
Write-Host ""

if ($FieldFail -gt 0) {
    Write-Host "field 门禁未通过 — v1.0 不可交付。" -ForegroundColor Red
    exit 1
}
Write-Host "field 门禁通过 — 可将 step11 标为完成（reboot 项仍须手工确认）。" -ForegroundColor Green
