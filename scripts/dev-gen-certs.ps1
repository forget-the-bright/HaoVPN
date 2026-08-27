# 生成开发用自签 CA + 服务端证书 → ./certs/
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$CertDir = if ($args[0]) { $args[0] } else { "./certs" }
$Days = 3650

if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    Write-Error "需要 openssl（可安装 Git for Windows 或 OpenSSL）"
}

New-Item -ItemType Directory -Force -Path $CertDir | Out-Null

Write-Host "==> 生成 CA"
openssl genrsa -out "$CertDir/ca.key" 4096 2>$null
openssl req -new -x509 -days $Days -key "$CertDir/ca.key" -out "$CertDir/ca.crt" `
  -subj "/CN=HaoVPN Dev CA"

Write-Host "==> 生成服务端证书"
openssl genrsa -out "$CertDir/server.key" 2048 2>$null
openssl req -new -key "$CertDir/server.key" -out "$CertDir/server.csr" `
  -subj "/CN=HaoVPN-dev.local"
openssl x509 -req -days $Days -in "$CertDir/server.csr" `
  -CA "$CertDir/ca.crt" -CAkey "$CertDir/ca.key" -CAcreateserial `
  -out "$CertDir/server.crt"

Remove-Item -Force "$CertDir/server.csr" -ErrorAction SilentlyContinue
Remove-Item -Force "$CertDir/ca.srl" -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "完成:"
Write-Host "  CA:     $CertDir/ca.crt"
Write-Host "  服务端: $CertDir/server.crt / $CertDir/server.key"
