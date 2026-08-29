#Requires -Version 7.0
<#
.SYNOPSIS
  打包「公司电脑客户端验证」目录与 zip：二进制 + 测试账号文档 + 诊断脚本 + client.yaml。

.EXAMPLE
  .\scripts\pack-company-client-test.ps1
  .\scripts\pack-company-client-test.ps1 -ServerAddress "192.168.196.17:8443"
#>
param(
    [string]$ServerAddress = "192.168.196.17:8443",
    [string]$TestUser = "company_test",
    [string]$TestPass = "CompanyTest@2026",
    [string]$ApiBase = "http://127.0.0.1:8080",
    [string]$AdminUser = "admin",
    [string]$AdminPass = "changeme123"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

Write-Host "==> 确保测试账号 $TestUser …"
& go run ./scripts/ensure_company_test_user
if ($LASTEXITCODE -ne 0) {
    Write-Host "警告: 未能写入测试账号（数据库可能被服务端占用）。若服务端已在跑，请用 WebUI 确认账号存在。" -ForegroundColor Yellow
}

Write-Host "==> build-local…"
& "$PSScriptRoot/build-local.ps1" | Out-Null

$ver = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim()
$stamp = Get-Date -Format "yyyyMMdd-HHmm"
$OutDir = Join-Path $Root "dist\company-client-test"
$Zip = Join-Path $Root "dist\haovpn-company-client-test-$stamp.zip"

if (Test-Path $OutDir) { Remove-Item -Recurse -Force $OutDir }
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutDir "bin") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutDir "logs") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutDir "certs") -Force | Out-Null

Copy-Item -Force (Join-Path $Root "bin\haovpn-client.exe") (Join-Path $OutDir "bin\")
if (Test-Path (Join-Path $Root "bin\haovpn-client-gui.exe")) {
    Copy-Item -Force (Join-Path $Root "bin\haovpn-client-gui.exe") (Join-Path $OutDir "bin\")
}
# wintun 已内嵌于 exe，首次连 TUN 时释放到 exe 同目录，无需单独拷贝
if (Test-Path (Join-Path $Root "home\certs\server.crt")) {
    Copy-Item -Force (Join-Path $Root "home\certs\server.crt") (Join-Path $OutDir "certs\")
}

# 尝试从本机服务端导出（有则覆盖下面的手写 yaml）
$exported = $false
$userID = 0
try {
    $ws = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $login = Invoke-WebRequest -Uri "$ApiBase/api/v1/login" -Method POST -Body @{
        username = $AdminUser
        password = $AdminPass
    } -WebSession $ws -UseBasicParsing -TimeoutSec 5
    $csrf = ($login.Content | ConvertFrom-Json).csrf_token
    $expHeaders = @{ "X-CSRF-Token" = $csrf }
    $users = Invoke-WebRequest -Uri "$ApiBase/api/v1/users" -WebSession $ws -UseBasicParsing
    $list = $users.Content | ConvertFrom-Json
    $hit = $list | Where-Object { $_.username -eq $TestUser } | Select-Object -First 1
    if ($hit) {
        $userID = [int64]$hit.id
        $zipPath = Join-Path $OutDir "account-export.zip"
        Invoke-WebRequest -Uri "$ApiBase/api/v1/users/$userID/export.zip" -Method POST -Headers $expHeaders -WebSession $ws -OutFile $zipPath -UseBasicParsing
        Expand-Archive -Path $zipPath -DestinationPath (Join-Path $OutDir "account-export") -Force
        $yamlPath = Join-Path $OutDir "account-export\client.yaml"
        if (Test-Path $yamlPath) {
            $yaml = Get-Content $yamlPath -Raw -Encoding utf8
            $yaml = $yaml -replace 'address:\s*"[^"]+"', ("address: `"{0}`"" -f $ServerAddress)
            $yaml = $yaml -replace 'ca_file:\s*"[^"]+"', 'ca_file: "./certs/server.crt"'
            $yaml = $yaml -replace 'file:\s*"[^"]*client\.log"', 'file: "./logs/client.log"'
            Set-Content -Path (Join-Path $OutDir "client.yaml") -Value $yaml -Encoding utf8
            $exported = $true
            $expCert = Join-Path $OutDir "account-export\certs\server.crt"
            if (Test-Path $expCert) {
                Copy-Item -Force $expCert (Join-Path $OutDir "certs\server.crt")
            }
        }
        Write-Host "    已导出 user=$TestUser id=$userID → client.yaml (address=$ServerAddress)"
    }
} catch {
    Write-Host "    提示: API 未导出（$($_.Exception.Message)），将写入内置 client.yaml" -ForegroundColor Yellow
}

if (-not $exported) {
    $fallback = @"
# 公司机测试客户端配置（打包生成）
server:
  address: "$ServerAddress"
  tls:
    ca_file: "./certs/server.crt"
    insecure_skip_verify: false
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90

tun:
  name: "haovpn0"
  mtu: 1420
  dns_from_policy: true

security:
  kill_switch: false

auth:
  username: "$TestUser"
  # 密码见 TEST-ACCOUNT.md；勿写在此处

reconnect:
  initial_sec: 1
  max_sec: 8

log:
  level: "info"
  file: "./logs/client.log"
  max_size_mb: 50
  max_backups: 3
"@
    Set-Content -Path (Join-Path $OutDir "client.yaml") -Value $fallback -Encoding utf8
    Write-Host "    已写入内置 client.yaml (user=$TestUser address=$ServerAddress)"
}

# 说明与脚本
Copy-Item -Force (Join-Path $PSScriptRoot "company-client\VERIFY.md") (Join-Path $OutDir "VERIFY.md")
Copy-Item -Force (Join-Path $PSScriptRoot "company-client\TEST-ACCOUNT.md") (Join-Path $OutDir "TEST-ACCOUNT.md")
Copy-Item -Force (Join-Path $PSScriptRoot "company-client\collect-client-info.ps1") (Join-Path $OutDir "collect-client-info.ps1")
Copy-Item -Force (Join-Path $PSScriptRoot "company-client\RESULT-TEMPLATE.md") (Join-Path $OutDir "RESULT-TEMPLATE.md")

$meta = @"
# 打包信息
- 版本: $ver
- 打包时间: $(Get-Date -Format "yyyy-MM-dd HH:mm:ss")
- 测试账号: $TestUser （密码见 TEST-ACCOUNT.md）
- 自动导出账号: $exported (user_id=$userID)
- server.address 预填: $ServerAddress
- 客户端: bin/haovpn-client.exe、bin/haovpn-client-gui.exe
- 阅读顺序: TEST-ACCOUNT.md → VERIFY.md → 测试 → RESULT-TEMPLATE.md + collect-client-info.ps1
"@
Set-Content -Path (Join-Path $OutDir "PACK-INFO.txt") -Value $meta -Encoding utf8

if (Test-Path $Zip) { Remove-Item -Force $Zip }
Compress-Archive -Path (Join-Path $OutDir "*") -DestinationPath $Zip -Force

Write-Host ""
Write-Host "==> 公司机验证包已就绪" -ForegroundColor Green
Write-Host "    目录: $OutDir"
Write-Host "    ZIP:  $Zip"
Write-Host "    账号: $TestUser / （见包内 TEST-ACCOUNT.md）"
Write-Host "    地址: $ServerAddress"
Write-Host "    下一步: 拷到公司电脑，先读 TEST-ACCOUNT.md 与 VERIFY.md"
