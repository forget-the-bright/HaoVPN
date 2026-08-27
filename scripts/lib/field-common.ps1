# field-common.ps1 — 现场交付门禁共用函数（require_tun + NAT 真路径）

function New-FieldRecord {
    param($Name, $Ok, $Detail = "")
    if ($Ok) {
        $script:FieldPass++
        $script:FieldResults.Add("[PASS] $Name")
        Write-Host "  [PASS] $Name" -ForegroundColor Green
    } else {
        $script:FieldFail++
        $msg = if ($Detail) { "$Name — $Detail" } else { $Name }
        $script:FieldResults.Add("[FAIL] $msg")
        Write-Host "  [FAIL] $msg" -ForegroundColor Red
    }
}

function New-FieldServerYaml {
    param(
        [string]$Dir,
        [int]$ApiPort,
        [int]$TunnelPort,
        [string]$LanCidr
    )
    # 路径必须绝对化：服务端 WorkingDirectory 为 bin/，相对路径会写到错误目录
    $absDir = (Resolve-Path -LiteralPath $Dir).Path
    $dirUnix = ($absDir -replace '\\', '/')
    return @"
server:
  listen: "127.0.0.1:$TunnelPort"
  tls:
    cert_file: "$dirUnix/certs/server.crt"
    key_file: "$dirUnix/certs/server.key"
    auto_generate: true
vpn:
  subnet: "10.88.0.0/24"
  gateway_ip: "10.88.0.1"
  mtu: 1420
  heartbeat_timeout_sec: 30
  require_tun: true
nat:
  enabled: true
  allowed_lan_cidrs:
    - "$LanCidr"
database:
  path: "$dirUnix/data/haovpn.db"
  encryption_key: ""
api:
  listen_hosts: ["127.0.0.1"]
  port: $ApiPort
  allow_public_bind: false
  login_max_attempts: 5
  login_lockout_sec: 60
security:
  enforce_split_tunnel: true
admin:
  username: "admin"
  password: "changeme123"
log:
  level: "info"
  file: "$dirUnix/logs/server.log"
"@
}

function Test-FieldIsAdmin {
    return ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Start-FieldElevatedProcess {
    param(
        [string]$Exe,
        [string[]]$ArgList,
        [string]$WorkDir
    )
    if (Test-FieldIsAdmin) {
        return Start-Process -FilePath $Exe -ArgumentList $ArgList -WorkingDirectory $WorkDir -PassThru -WindowStyle Hidden
    }
    $sudo = Join-Path $env:SystemRoot "System32\sudo.exe"
    if (-not (Test-Path $sudo)) { $sudo = "sudo" }
    return Start-Process -FilePath $sudo -ArgumentList (@($Exe) + $ArgList) -WorkingDirectory $WorkDir -PassThru -WindowStyle Hidden
}

function Start-FieldServer {
    param([string]$YamlPath, [string]$Root)
    $exe = Join-Path $Root "bin\haovpn-server.exe"
    $wd = Join-Path $Root "bin"
    $yamlAbs = (Resolve-Path -LiteralPath $YamlPath).Path
    return Start-FieldElevatedProcess $exe @("-c", $yamlAbs) $wd
}

function Get-FieldServerProcess {
    return Get-Process -Name "haovpn-server" -ErrorAction SilentlyContinue | Select-Object -First 1
}

function Wait-FieldServerProcess {
    param([int]$Sec = 20)
    for ($i = 0; $i -lt $Sec; $i++) {
        $p = Get-FieldServerProcess
        if ($p) { return $p }
        Start-Sleep -Seconds 1
    }
    return $null
}

function Wait-FieldHealth {
    param([int]$Port, [int]$Sec = 30, [switch]$RequireTunNat)
    $last = $null
    $lastErr = ""
    for ($i = 0; $i -lt $Sec; $i++) {
        Start-Sleep -Seconds 1
        try {
            $r = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/v1/health" -TimeoutSec 2
            $last = $r
            if (-not $r.db_ok) { continue }
            if ($RequireTunNat) {
                if ($r.tun_ok -and $r.nat_ok) { return $r }
            } else {
                return $r
            }
        } catch {
            $lastErr = $_.Exception.Message
        }
    }
    if ($last) {
        $script:FieldHealthLast = $last
    } else {
        $script:FieldHealthLast = @{ error = $lastErr }
    }
    return $null
}

function Stop-FieldProc {
    param($Proc)
    if ($Proc -and -not $Proc.HasExited) {
        Stop-Process -Id $Proc.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 800
    }
}

function Start-FieldClient {
    param([string]$ClientYaml, [string]$Root)
    $exe = Join-Path $Root "bin\haovpn-client.exe"
    $wd = Join-Path $Root "bin"
    $yamlAbs = (Resolve-Path -LiteralPath $ClientYaml).Path
    return Start-FieldElevatedProcess $exe @("-c", $yamlAbs) $wd
}

function Read-LiveLogText {
    param([string]$LogFile)
    $candidates = @(
        $LogFile,
        ($LogFile -replace '\.log$', '.live.log')
    )
    # 兼容误写到 bin/ 下的相对路径日志
    $base = Split-Path $LogFile -Leaf
    $liveBase = $base -replace '\.log$', '.live.log'
    $binRoot = Join-Path (Split-Path (Split-Path $LogFile -Parent) -Parent) "bin"
    if (Test-Path $binRoot) {
        $relDir = Split-Path $LogFile -Parent | Split-Path -Leaf
        $candidates += @(
            (Join-Path $binRoot "$relDir\logs\$base"),
            (Join-Path $binRoot "$relDir\logs\$liveBase")
        )
    }
    foreach ($p in ($candidates | Select-Object -Unique)) {
        if ($p -and (Test-Path $p)) {
            $txt = Get-Content $p -Raw -ErrorAction SilentlyContinue
            if ($txt) { return $txt }
        }
    }
    return ""
}

function Assert-FieldServerLiveLog {
    param([string]$LogText)
    if (-not $LogText -or $LogText.Trim().Length -lt 20) {
        return @{ Tun = $false; Nat = $false; NoFake = $true; NoTunFail = $true; Empty = $true }
    }
    $tunOk = ($LogText -match "windows TUN IP 已配置") -or ($LogText -match "TUN IP 已配置")
    $natOk = ($LogText -match "New-NetNat") -or ($LogText -match "MASQUERADE") -or ($LogText -match "ICS 已启用")
    $noFake = ($LogText -notmatch "ICS/NAT setup") -and ($LogText -notmatch "manual/route command may needed")
    $noTunFail = $LogText -notmatch "TUN 创建失败"
    return @{ Tun = $tunOk; Nat = $natOk; NoFake = $noFake; NoTunFail = $noTunFail; Empty = $false }
}

function Invoke-FieldLogin {
    param([string]$Base)
    $ws = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $lr = Invoke-WebRequest -Uri "$Base/api/v1/login" -Method POST -Body @{
        username = "admin"; password = "changeme123"
    } -WebSession $ws -UseBasicParsing
    $csrf = ($lr.Content | ConvertFrom-Json).csrf_token
    return @{ Session = $ws; CSRF = $csrf }
}

function Expand-FieldClientZip {
    param([string]$ZipPath, [string]$OutDir, [string]$ServerAddr)
    if (Test-Path $OutDir) { Remove-Item -Recurse -Force $OutDir }
    New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
    Expand-Archive -Path $ZipPath -DestinationPath $OutDir -Force
    $yamlPath = Join-Path $OutDir "client.yaml"
    if (-not (Test-Path $yamlPath)) { throw "zip 缺少 client.yaml" }
    $yaml = Get-Content $yamlPath -Raw
    $yaml = $yaml -replace 'address: "REPLACE_WITH_SERVER_IP:[^"]+"', "address: `"$ServerAddr`""
    $yaml = $yaml -replace 'address: "127\.0\.0\.1:\d+"', "address: `"$ServerAddr`""
    Set-Content -Path $yamlPath -Value $yaml -Encoding UTF8
    return $yamlPath
}
