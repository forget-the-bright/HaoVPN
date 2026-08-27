#Requires -Version 7.0
<#
.SYNOPSIS
  公司机一键收集客户端验证诊断信息（打包成 diag-*.zip）。
.EXAMPLE
  pwsh -File .\collect-client-info.ps1
#>
$ErrorActionPreference = "Continue"
$Root = $PSScriptRoot
Set-Location $Root

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$DiagDir = Join-Path $Root "diag-$stamp"
New-Item -ItemType Directory -Path $DiagDir -Force | Out-Null

function Copy-IfExists($src, $dstName) {
    if (Test-Path $src) {
        Copy-Item -Force $src (Join-Path $DiagDir $dstName)
        Write-Host "  + $dstName"
    } else {
        Write-Host "  - missing: $src"
    }
}

Write-Host "==> 收集中…"

Copy-IfExists (Join-Path $Root "client.yaml") "client.yaml"
Copy-IfExists (Join-Path $Root "PACK-INFO.txt") "PACK-INFO.txt"
Copy-IfExists (Join-Path $Root "RESULT-TEMPLATE.md") "RESULT-TEMPLATE.md"
# 不收集 TEST-ACCOUNT.md（含测试密码），避免诊断包外传泄漏
$rt = Join-Path $Root "RESULT-TEMPLATE.md"
if (Test-Path $rt) {
    $raw = Get-Content $rt -Raw -ErrorAction SilentlyContinue
    if ($raw -match "填写日期：_{2,}") {
        Write-Host "  ! 提醒: RESULT-TEMPLATE.md 似乎尚未填写，请填完再带回" -ForegroundColor Yellow
    }
}

# 日志可能写在多个位置
$logCandidates = @(
    (Join-Path $Root "logs\client.log"),
    (Join-Path $Root "logs\client.live.log"),
    (Join-Path $Root "bin\logs\client.log"),
    (Join-Path $Root "bin\logs\client.live.log"),
    (Join-Path $Root "account-export\logs\client.log")
)
$i = 0
foreach ($p in $logCandidates) {
    if (Test-Path $p) {
        $i++
        Copy-Item -Force $p (Join-Path $DiagDir ("log-{0}-{1}" -f $i, (Split-Path $p -Leaf)))
        Write-Host "  + log: $p"
    }
}

# 系统与网络摘要（不含敏感公司内网扫全网）
$sys = @()
$sys += "=== time ==="
$sys += (Get-Date).ToString("o")
$sys += "=== os ==="
$sys += (Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber | Format-List | Out-String)
$sys += "=== is admin ==="
$sys += ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
$sys += "=== client exe ==="
if (Test-Path (Join-Path $Root "bin\haovpn-client.exe")) {
    $fi = Get-Item (Join-Path $Root "bin\haovpn-client.exe")
    $sys += "path=$($fi.FullName) size=$($fi.Length) mtime=$($fi.LastWriteTime)"
    try {
        $sys += (& (Join-Path $Root "bin\haovpn-client.exe") -version 2>&1 | Out-String)
    } catch {
        $sys += "version flag failed: $($_.Exception.Message)"
    }
}
$sys += "=== adapters (name/status/ip) ==="
try {
    $sys += (Get-NetIPConfiguration | Select-Object InterfaceAlias, IPv4Address | Format-List | Out-String)
} catch {
    $sys += $_.Exception.Message
}
$sys += "=== routes containing 10.88 ==="
try {
    $sys += (route print | Select-String "10\.88" | Out-String)
} catch {
    $sys += $_.Exception.Message
}
$sys += "=== ping 10.88.0.1 ==="
try {
    $sys += (ping -n 4 10.88.0.1 | Out-String)
} catch {
    $sys += $_.Exception.Message
}

# 从 client.yaml 抽 address（不打印整份私钥）
$yamlPath = Join-Path $Root "client.yaml"
if (Test-Path $yamlPath) {
    $sys += "=== client.yaml address / gateway (redacted keys) ==="
    Get-Content $yamlPath | ForEach-Object {
        if ($_ -match 'private_key\s*:') {
            $sys += "  private_key: `"***redacted***`""
        } elseif ($_ -match 'public_key\s*:') {
            $sys += "  public_key: `"***redacted***`""
        } else {
            $sys += $_
        }
    }
}

Set-Content -Path (Join-Path $DiagDir "system-summary.txt") -Value ($sys -join "`n") -Encoding utf8
Write-Host "  + system-summary.txt"

$zip = Join-Path $Root "diag-$stamp.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $DiagDir "*") -DestinationPath $zip -Force

Write-Host ""
Write-Host "==> 诊断包已生成:" -ForegroundColor Green
Write-Host "    $zip"
Write-Host "请把该 zip 与已填写的 RESULT-TEMPLATE.md 一起带回（密码见 TEST-ACCOUNT.md，勿放进 diag）。"
