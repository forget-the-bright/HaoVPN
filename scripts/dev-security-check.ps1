#Requires -Version 7.0
<#
.SYNOPSIS
  检查 server.yaml 安全配置（生产交付前运行，同 dev-security-check.sh）

.EXAMPLE
  .\scripts\dev-security-check.ps1
  .\scripts\dev-security-check.ps1 .\server.yaml
#>
param([string]$ConfigPath = "./server.yaml")

$ErrorActionPreference = "Stop"
if (-not (Test-Path $ConfigPath)) {
    Write-Error "配置文件不存在: $ConfigPath"
}

$text = Get-Content $ConfigPath -Raw
$fail = 0

function Warn($msg) { Write-Host "  [WARN] $msg"; $fail = 1 }
function Ok($msg)   { Write-Host "  [OK]   $msg" }

Write-Host "==> 安全检查: $ConfigPath"

if ($text -match 'allow_public_bind:\s*true') {
    Warn "allow_public_bind 为 true（生产应为 false）"
} else {
    Ok "allow_public_bind 未开启或为 false"
}

if ($text -match '0\.0\.0\.0') {
    if ($text -match 'allow_public_bind:\s*true') {
        Warn "listen_hosts 含 0.0.0.0 且 allow_public_bind=true（仅开发）"
    } else {
        Ok "listen_hosts 含 0.0.0.0 但 allow_public_bind=false（启动应拒绝）"
    }
} else {
    Ok "listen_hosts 未绑定 0.0.0.0"
}

if ($text -match 'password:\s*"?changeme"?') {
    Warn "admin 密码仍为 changeme"
} else {
    Ok "admin 默认密码已修改或不在配置中"
}

if ($text -match 'insecure_skip_verify:\s*true') {
    Warn "存在 insecure_skip_verify: true"
} else {
    Ok "未跳过 TLS 校验"
}

Write-Host ""
if ($fail -eq 0) {
    Write-Host "安全检查通过"
} else {
    Write-Host "存在警告项，请逐项确认"
    exit 1
}
