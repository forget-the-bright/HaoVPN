#Requires -Version 7.0
<#
.SYNOPSIS
  v1.0 smoke 验收（无管理员，require_tun=false）— 通过 ≠ v1.0 可交付

.EXAMPLE
  .\scripts\dev-acceptance.ps1
  # 现场交付硬门禁（TUN+NAT+PLC+服务）:
  .\scripts\dev-field-gate.ps1 -PlcHost 192.168.1.10 -UseHomeConfig
#>
param()

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$Pass = 0
$Fail = 0
$Skip = 0
$Results = [System.Collections.Generic.List[string]]::new()

function Record($name, $ok, $detail = "") {
    if ($ok) {
        $script:Pass++
        $script:Results.Add("[PASS] $name")
        Write-Host "  [PASS] $name" -ForegroundColor Green
    } else {
        $script:Fail++
        $msg = if ($detail) { "$name — $detail" } else { $name }
        $script:Results.Add("[FAIL] $msg")
        Write-Host "  [FAIL] $msg" -ForegroundColor Red
    }
}

function RecordSkip($name, $why) {
    $script:Skip++
    $script:Results.Add("[SKIP] $name — $why")
    Write-Host "  [SKIP] $name ($why)" -ForegroundColor Yellow
}

function New-ServerYaml($dir, $apiPort, $tunnelPort, $listenHosts, $allowPublic) {
    $dirUnix = ($dir -replace '\\', '/')
    $hostsYaml = ($listenHosts | ForEach-Object { "`"$($_)`"" }) -join ', '
    return @"
server:
  listen: "127.0.0.1:$tunnelPort"
  tls:
    cert_file: "$dirUnix/certs/server.crt"
    key_file: "$dirUnix/certs/server.key"
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
  path: "$dirUnix/data/haovpn.db"
api:
  listen_hosts: [$hostsYaml]
  port: $apiPort
  allow_public_bind: $($allowPublic.ToString().ToLower())
  login_max_attempts: 5
  login_lockout_sec: 60
security:
  enforce_split_tunnel: true
admin:
  username: "admin"
  password: "changeme123"
  sync_password_from_config: true
log:
  level: "info"
  file: "$dirUnix/logs/server.log"
"@
}

function Start-Server($yamlPath, $useSudo) {
    $exe = Join-Path $Root "bin\haovpn-server.exe"
    $wd = Join-Path $Root "bin"
    if ($useSudo) {
        return Start-Process -FilePath "sudo" -ArgumentList @($exe, "-c", $yamlPath) -WorkingDirectory $wd -PassThru -WindowStyle Hidden
    }
    return Start-Process -FilePath $exe -ArgumentList @("-c", $yamlPath) -WorkingDirectory $wd -PassThru -WindowStyle Hidden
}

function Wait-Health($port, $sec = 20) {
    for ($i = 0; $i -lt $sec; $i++) {
        Start-Sleep -Seconds 1
        try {
            $r = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/v1/health" -TimeoutSec 2
            if ($r.db_ok) { return $true }
        } catch { }
    }
    return $false
}

function Stop-Proc($proc) {
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500
    }
}

Write-Host "========================================"
Write-Host "  HaoVPN v1.0 本地验收测试"
Write-Host "========================================"

# --- 1. 单元测试 ---
Write-Host "`n==> [1] 单元测试 go test ./..."
go test ./... -count=1
if ($LASTEXITCODE -eq 0) { Record "go test ./..." $true } else { Record "go test ./..." $false }

# --- 2. 构建 ---
Write-Host "`n==> [2] 本机构建"
& "$PSScriptRoot/build-local.ps1" | Out-Null
if (Test-Path (Join-Path $Root "bin\haovpn-server.exe")) { Record "build-local" $true } else { Record "build-local" $false }

$AcceptDir = Join-Path $Root "accept-tmp"
if (Test-Path $AcceptDir) { Remove-Item -Recurse -Force $AcceptDir }
New-Item -ItemType Directory -Path $AcceptDir -Force | Out-Null

$ServerExe = Join-Path $Root "bin\haovpn-server.exe"

# --- 3. 安全清单 #1：误配拒绝启动 ---
Write-Host "`n==> [3] 安全清单（配置层）"
$badYaml = Join-Path $AcceptDir "bad-bind.yaml"
New-ServerYaml $AcceptDir 19080 19443 @("0.0.0.0") $false | Set-Content $badYaml -Encoding UTF8
$badProc = Start-Process -FilePath $ServerExe -ArgumentList @("-c", $badYaml) -WorkingDirectory (Join-Path $Root "bin") -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2
$rejected = $badProc.HasExited -and $badProc.ExitCode -ne 0
if (-not $badProc.HasExited) { Stop-Proc $badProc }
Record "安全#1 0.0.0.0+false 拒绝启动" $rejected

# --- 4. 启动验收服务端 ---
Write-Host "`n==> [4] 启动虚拟验收环境"
$mainYaml = Join-Path $AcceptDir "server.yaml"
New-ServerYaml $AcceptDir 19080 19443 @("127.0.0.1") $false | Set-Content $mainYaml -Encoding UTF8
$apiPort = 19080
$tunnelPort = 19443
$serverProc = Start-Server $mainYaml $false
if (-not (Wait-Health $apiPort)) {
    Record "服务端启动与健康检查" $false "health 超时"
    Write-Host "`n验收中止：服务端未就绪"
    exit 1
}
Record "服务端启动与健康检查" $true

$base = "http://127.0.0.1:$apiPort"
$ws = New-Object Microsoft.PowerShell.Commands.WebRequestSession

# --- 5. API 验收流程 ---
Write-Host "`n==> [5] API / WebUI 验收流程"

try {
    $exp = Invoke-WebRequest -Uri "$base/api/v1/users/1/export" -UseBasicParsing -ErrorAction Stop
    Record "安全#5 未登录导出" $false "应 401，得 $($exp.StatusCode)"
} catch {
    $code = $_.Exception.Response.StatusCode.value__
    Record "安全#5 未登录导出 401" ($code -eq 401)
}

$login = Invoke-WebRequest -Uri "$base/api/v1/login" -Method POST -Body @{
    username = "admin"
    password = "changeme123"
} -WebSession $ws -UseBasicParsing
$loginJson = $login.Content | ConvertFrom-Json
$csrf = $loginJson.csrf_token
Record "登录 API" ($login.StatusCode -eq 200 -and $csrf)

$health = Invoke-RestMethod -Uri "$base/api/v1/health"
Record "健康探针 db_ok" ($health.db_ok -eq $true)

$headers = @{ "X-CSRF-Token" = $csrf }
$userResp = Invoke-WebRequest -Uri "$base/api/v1/users" -Method POST -Body @{
    username = "engineer_accept"
    password = "AcceptTest123!"
    ip_mode  = "fixed"
} -Headers $headers -WebSession $ws -UseBasicParsing
$userJson = $userResp.Content | ConvertFrom-Json
$userId = $userJson.id
Record "创建 VPN 账号分配 IP" ($userId -gt 0 -and $userJson.vpn_ip)

$export = Invoke-WebRequest -Uri "$base/api/v1/users/$userId/export" -Method POST -Headers $headers -WebSession $ws -UseBasicParsing
if ($export.Content -is [byte[]]) {
    $exportText = [System.Text.Encoding]::UTF8.GetString($export.Content)
} else {
    $exportText = [string]$export.Content
}
$hasAuth = $exportText -match 'username:' -and $exportText -match 'auth:'
$noPrivateKey = $exportText -notmatch 'private_key:'
$noAdminPwd = $exportText -notmatch 'changeme123'
Record "导出客户端 YAML" ($export.StatusCode -eq 200 -and $hasAuth -and $noPrivateKey -and $noAdminPwd) "status=$($export.StatusCode) hasAuth=$hasAuth noPrivateKey=$noPrivateKey noPwd=$noAdminPwd len=$($exportText.Length)"

$zipPath = Join-Path $AcceptDir "client-export.zip"
Invoke-WebRequest -Uri "$base/api/v1/users/$userId/export.zip" -Method POST -Headers $headers -WebSession $ws -OutFile $zipPath -UseBasicParsing
$zipOk = (Test-Path $zipPath) -and ((Get-Item $zipPath).Length -gt 100)
Record "导出客户端 zip" $zipOk "bytes=$(if(Test-Path $zipPath){(Get-Item $zipPath).Length}else{0})"

$logs = Invoke-RestMethod -Uri "$base/api/v1/logs?tail=50" -WebSession $ws
Record "日志快照 API" ($null -ne $logs.lines)

$monitor = Invoke-RestMethod -Uri "$base/api/v1/monitor/accounts" -WebSession $ws
Record "Monitor API" ($null -ne $monitor)

# 禁用账号应踢线（API 层）
$disableFd = @{ action = "disable" }
$disableUser = Invoke-WebRequest -Uri "$base/api/v1/users/$userId" -Method POST -Body $disableFd -Headers $headers -WebSession $ws -UseBasicParsing
$userAfter = (Invoke-RestMethod -Uri "$base/api/v1/users" -WebSession $ws) | Where-Object { $_.id -eq $userId }
Record "禁用账号 API" ($disableUser.StatusCode -eq 200 -and $userAfter.enabled -eq $false)

$audit = Invoke-RestMethod -Uri "$base/api/v1/audit" -WebSession $ws
$auditText = ($audit | ConvertTo-Json -Depth 5)
Record "审计含 account_create/export" (
    $auditText -match "account_create" -and $auditText -match "config_export"
)

foreach ($page in @("/", "/users", "/connections", "/audit", "/login")) {
    $p = Invoke-WebRequest -Uri "$base$page" -WebSession $ws -UseBasicParsing
    $ok = $p.StatusCode -eq 200
    if ($page -eq "/login") {
        $p2 = Invoke-WebRequest -Uri "$base/login" -UseBasicParsing
        $ok = $p2.StatusCode -eq 200
    }
    Record "WebUI 页面 $page" $ok
}

# /peers 重定向到 /users
try {
    $peersRedirect = Invoke-WebRequest -Uri "$base/peers" -WebSession $ws -MaximumRedirection 0 -ErrorAction Stop
    Record "WebUI /peers 重定向" ($peersRedirect.StatusCode -eq 302)
} catch {
    $code = $_.Exception.Response.StatusCode.value__
    Record "WebUI /peers 重定向" ($code -eq 302)
}

# 安全响应头
$h = Invoke-WebRequest -Uri "$base/api/v1/health" -UseBasicParsing
Record "安全响应头 nosniff" ($h.Headers["X-Content-Type-Options"] -eq "nosniff")

# CSRF #9（独立会话，不带 CSRF 头）
$csrfWs = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$csrfLogin = Invoke-WebRequest -Uri "$base/api/v1/login" -Method POST -Body @{
    username = "admin"
    password = "changeme123"
} -WebSession $csrfWs -UseBasicParsing
try {
    Invoke-WebRequest -Uri "$base/api/v1/logout" -Method POST -WebSession $csrfWs -UseBasicParsing -ErrorAction Stop | Out-Null
    Record "安全#9 CSRF 无 token 403" $false "无 CSRF 仍返回 200"
} catch {
    $code = $_.Exception.Response.StatusCode.value__
    Record "安全#9 CSRF 无 token 403" ($code -eq 403) "code=$code"
}

# 登录锁定 #4（新会话，避免污染；多试几次确保触发 max_attempts=5）
$lockWs = New-Object Microsoft.PowerShell.Commands.WebRequestSession
for ($i = 0; $i -lt 6; $i++) {
    try {
        Invoke-WebRequest -Uri "$base/api/v1/login" -Method POST -Body @{
            username = "admin"
            password = "wrongpass"
        } -WebSession $lockWs -UseBasicParsing -ErrorAction Stop | Out-Null
    } catch { }
}
$lockHit = $false
try {
    Invoke-WebRequest -Uri "$base/api/v1/login" -Method POST -Body @{
        username = "admin"
        password = "changeme123"
    } -WebSession $lockWs -UseBasicParsing -ErrorAction Stop | Out-Null
    Record "安全#4 登录锁定" $false "正确密码仍应被拒绝"
} catch {
    $body = $_.ErrorDetails.Message
    if (-not $body -and $_.Exception.Response) {
        $reader = [System.IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
        $body = $reader.ReadToEnd()
        $reader.Close()
    }
    $lockHit = $false
    if ($body) {
        try {
            $errJson = $body | ConvertFrom-Json
            if ($errJson.error) { $body = [string]$errJson.error }
        } catch { }
        $lockHit = ($body -match "稍后再试" -or $body -match "过多")
    }
    Record "安全#4 连续错密锁定" $lockHit "body=$body"
}

# 日志无明文密码 #7
$logPath = Join-Path $AcceptDir "logs\server.log"
if (Test-Path $logPath) {
    $logText = Get-Content $logPath -Raw
    $noPwd = $logText -notmatch "changeme123" -and $logText -notmatch "password=changeme"
    Record "安全#7 日志无明文密码" $noPwd
} else {
    RecordSkip "安全#7 日志检查" "日志文件不存在"
}

# --- 6. allow_public_bind=true 审计记录 #2 ---
Write-Host "`n==> [6] 安全#2 公网绑定审计"
Stop-Proc $serverProc
$pubYaml = Join-Path $AcceptDir "server-public.yaml"
New-ServerYaml $AcceptDir 19081 19444 @("0.0.0.0") $true | Set-Content $pubYaml -Encoding UTF8
$pubProc = Start-Server $pubYaml $false
if (Wait-Health 19081 15) {
    $pubLog = Join-Path $AcceptDir "logs\server.log"
    # 公网配置复用同 log 文件会追加；检查 audit API
    $pubWs = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    Invoke-WebRequest -Uri "http://127.0.0.1:19081/api/v1/login" -Method POST -Body @{
        username = "admin"; password = "changeme123"
    } -WebSession $pubWs -UseBasicParsing | Out-Null
    $pubAudit = Invoke-RestMethod -Uri "http://127.0.0.1:19081/api/v1/audit" -WebSession $pubWs
    $pubAuditText = ($pubAudit | ConvertTo-Json -Depth 5)
    Record "安全#2 public_bind 审计记录" ($pubAuditText -match "management_public_bind_enabled")
} else {
    Record "安全#2 public_bind 启动" $false
}
Stop-Proc $pubProc

# --- 7. 隧道握手 + crypto/安全门禁（go test）---
Write-Host "`n==> [7] 隧道握手与 P0 安全单测"
go test -count=1 -run "TestHandshakeIntegration|TestCrossSessionRoundTrip|TestReplayRejected|TestLateralVPNIPBlocked|TestIPPoolReloadFromPeers|TestKeyEncRoundTrip|TestGoSafeBusinessPathSurvivesPanic|TestPeerPrivateKeyAESAndExport|TestLogsAPIContainsMarker|TestMigratePlaintextPeerKeys|TestProbeMTUEnqueued|TestNewStatusIncludesRecentErrors|TestHandshakeReconnectNoDeadlock" ./internal/tunnel/... ./internal/crypto/... ./internal/sessionmgr/... ./internal/persist/... ./internal/security/... ./internal/safeutil/... ./internal/api/... ./internal/transport/... ./internal/health/...
if ($LASTEXITCODE -eq 0) { Record "P0 安全门禁 go test" $true } else { Record "P0 安全门禁 go test" $false }

go test -count=1 -run TestHandshakeIntegration ./internal/tunnel/...
if ($LASTEXITCODE -eq 0) { Record "隧道握手 E2E（无 TUN）" $true } else { Record "隧道握手 E2E" $false }

# --- 8. 现场交付请用 field 门禁（本脚本 smoke 不测 TUN/NAT/PLC）---
Write-Host "`n==> [8] 现场交付（field gate）"
RecordSkip "TUN+NAT+PLC+服务" "请运行: .\scripts\dev-field-gate.ps1 -PlcHost <工控IP> [-UseHomeConfig]"

Stop-Proc $serverProc

# 清理（先结束可能残留的验收进程）
Get-Process haovpn-server,haovpn-client -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Remove-Item -Recurse -Force $AcceptDir -ErrorAction SilentlyContinue

# --- 汇总 ---
Write-Host "`n========================================"
Write-Host "  验收汇总: PASS=$Pass  FAIL=$Fail  SKIP=$Skip"
Write-Host "========================================"
foreach ($line in $Results) {
    if ($line.StartsWith("[FAIL]")) { Write-Host $line -ForegroundColor Red }
    elseif ($line.StartsWith("[SKIP]")) { Write-Host $line -ForegroundColor Yellow }
}
Write-Host ""

if ($Fail -gt 0) {
    Write-Host "验收未通过，请查看 FAIL 项。" -ForegroundColor Red
    exit 1
}
Write-Host "smoke 验收通过（require_tun=false，不含 TUN/NAT/PLC）。" -ForegroundColor Green
Write-Host "v1.0 交付须另跑: .\scripts\dev-field-gate.ps1 -PlcHost <工控IP>" -ForegroundColor Yellow
