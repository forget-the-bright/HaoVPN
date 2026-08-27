#Requires -Version 7.0
<#
.SYNOPSIS
  v1.0 全量验证：单元测试 + E2E 冒烟 + 安全配置检查

.EXAMPLE
  .\scripts\dev-full-test.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

Write-Host "==> [1/3] go test ./..."
go test ./... -count=1
if ($LASTEXITCODE -ne 0) { throw "单元测试失败" }

Write-Host "==> [2/3] dev-e2e 冒烟"
& "$PSScriptRoot/dev-e2e.ps1"
if ($LASTEXITCODE -ne 0) { throw "E2E 失败" }

Write-Host "==> [3/3] 安全配置检查（合成安全配置）"
$safeCfg = Join-Path $env:TEMP "HaoVPN-security-check.yaml"
@"
api:
  listen_hosts: ["127.0.0.1"]
  port: 8080
  allow_public_bind: false
admin:
  username: admin
  password: SecurePass123!
server:
  listen: "127.0.0.1:8443"
"@ | Set-Content -Encoding utf8 $safeCfg
& "$PSScriptRoot/dev-security-check.ps1" $safeCfg

Write-Host ""
Write-Host "==> v1.0 全量验证通过"
