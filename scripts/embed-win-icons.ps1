# 生成 Windows .syso（嵌入 exe 图标）。需网络安装 go-winres（仅开发机一次）。
# 用法（仓库根目录）：
#   .\scripts\embed-win-icons.ps1
#
# 版本号一律读根目录 VERSION（见 docs/versioning.md），不在本脚本写死。
# 生成物：cmd/{server,client,client-gui}/rsrc_windows_*.syso
# 构建时 go build 会自动链接匹配 GOARCH 的 .syso。

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/lib/build-common.ps1"

$Root = Get-ProjectRoot
Set-Location $Root

$Version = Get-ProjectVersion -Root $Root
$VersionQuad = Get-VersionQuad -Version $Version
Write-Host "==> VERSION=$Version (quad=$VersionQuad)"
Sync-FyneAppTomlFromVersion -Root $Root -Version $Version

$iconSrc = Join-Path $Root "assets\appicon-256.png"
if (-not (Test-Path $iconSrc)) {
    Write-Host "==> 先生成图标"
    go run .\scripts\gen-icons.go
}
if (-not (Test-Path $iconSrc)) {
    $iconSrc = Join-Path $Root "assets\appicon.png"
}

$winresDir = Join-Path $Root "assets\winres"
Copy-Item -Force $iconSrc (Join-Path $winresDir "icon.png")

Write-Host "==> 安装/使用 go-winres"
$winres = Get-Command go-winres -ErrorAction SilentlyContinue
if (-not $winres) {
    go install github.com/tc-hib/go-winres@latest
    $gopathBin = Join-Path (go env GOPATH) "bin"
    $env:PATH = "$gopathBin;$env:PATH"
}

$pkgs = @(
    @{ Dir = "cmd\server"; Desc = "HaoVPN Server"; Exe = "haovpn-server.exe" },
    @{ Dir = "cmd\client"; Desc = "HaoVPN Client"; Exe = "haovpn-client.exe" },
    @{ Dir = "cmd\client-gui"; Desc = "HaoVPN"; Exe = "haovpn-client-gui.exe" }
)

foreach ($p in $pkgs) {
    $outDir = Join-Path $Root $p.Dir
    Write-Host "==> $($p.Dir)"
    $json = Get-Content (Join-Path $winresDir "winres.json") -Raw -Encoding UTF8
    $json = $json.Replace('{{VERSION}}', $Version)
    $json = $json.Replace('{{VERSION_QUAD}}', $VersionQuad)
    $json = $json.Replace('"FileDescription": "HaoVPN"', ('"FileDescription": "' + $p.Desc + '"'))
    $json = $json.Replace('"OriginalFilename": "haovpn.exe"', ('"OriginalFilename": "' + $p.Exe + '"'))
    $tmpJson = Join-Path $winresDir ("winres_" + ($p.Dir -replace '\\', '_') + ".json")
    Set-Content -Path $tmpJson -Value $json -Encoding UTF8

    Push-Location $outDir
    try {
        Get-ChildItem -Filter "rsrc_windows_*.syso" -ErrorAction SilentlyContinue | Remove-Item -Force
        & go-winres make --in $tmpJson --arch amd64,arm64 --out rsrc
        if ($LASTEXITCODE -ne 0) { throw "go-winres failed for $($p.Dir)" }
        Get-ChildItem -Filter "rsrc*.syso" | ForEach-Object { Write-Host "    $($_.Name)" }
    } finally {
        Pop-Location
    }
    Remove-Item $tmpJson -Force -ErrorAction SilentlyContinue
}

Write-Host "完成。重新 .\scripts\build-local.ps1 后 exe 资源管理器图标即生效。"
