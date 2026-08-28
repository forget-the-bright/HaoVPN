#Requires -Version 7.0

<#

.SYNOPSIS

  验证 CLI/GUI 单实例：持有锁时第二次启动应退出。



.EXAMPLE

  .\scripts\test-client-single-instance.ps1

#>

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot

Set-Location $Root



$exe = Join-Path $Root "bin\haovpn-client.exe"

if (-not (Test-Path $exe)) {

  Write-Host "请先 build-local: $exe 不存在" -ForegroundColor Red

  exit 1

}



go test ./internal/singleinstance/... -run "TestCLIAlreadyRunningExit|TestClientAlreadyRunning" -count=1 -v

if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }



Write-Host "`nCLI 单实例重复启动测试通过" -ForegroundColor Green

Write-Host @"

GUI 手工验收（Windows）：

  1. 启动一个 haovpn-client-gui，任务管理器应仅 1 个进程

  2. 再双击 3～5 次 → 「已在运行」→ 点确定后进程数仍为 1（不应再弹 UAC）

  3. 若旧版遗留多个 gui.exe：Stop-Process -Name haovpn-client-gui -Force

"@ -ForegroundColor Cyan

