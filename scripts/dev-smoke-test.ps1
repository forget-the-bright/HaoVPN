#Requires -Version 7.0
<#
.SYNOPSIS
  冒烟测试：构建本机二进制并运行 dev-e2e API 健康检查

.EXAMPLE
  .\scripts\dev-smoke-test.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

Write-Host "==> 本机构建"
& "$PSScriptRoot/build-local.ps1"
if ($LASTEXITCODE -ne 0) { throw "构建失败" }

Write-Host "==> E2E 冒烟"
& "$PSScriptRoot/dev-e2e.ps1"
if ($LASTEXITCODE -ne 0) { throw "E2E 失败" }

Write-Host "==> 冒烟通过"
