# 构建公共函数（供 build-release.ps1 / build-local.ps1 使用）
#Requires -Version 7.0

function Get-ProjectRoot {
    # scripts/lib/build-common.ps1 → 上两级为项目根
    return (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
}

function Get-ProjectVersion {
    param([string]$Root)
    $VersionFile = Join-Path $Root "VERSION"
    if (-not (Test-Path $VersionFile)) {
        throw "未找到 VERSION 文件。版本号仅由开发者维护，见 docs/versioning.md"
    }
    return (Get-Content $VersionFile -Raw).Trim()
}

function Get-GitCommitShort {
    param([string]$Root)
    Push-Location $Root
    try {
        $hash = git rev-parse --short HEAD 2>$null
        if ($LASTEXITCODE -eq 0 -and $hash) { return $hash.Trim() }
        return "unknown"
    } finally {
        Pop-Location
    }
}

function Get-BuildLdflags {
    param(
        [string]$Version,
        [string]$Commit,
        [string]$BuildTime
    )
    # 版本仅来自 VERSION 文件；commit/time 由脚本填充
    return "-s -w -X haovpn/internal/version.Version=$Version -X haovpn/internal/version.Commit=$Commit -X haovpn/internal/version.BuildTime=$BuildTime"
}

function Get-PlatformsFromFile {
    param([string]$Root)
    $File = Join-Path $Root "scripts/platforms.txt"
    if (-not (Test-Path $File)) {
        throw "未找到 scripts/platforms.txt"
    }
    $list = @()
    Get-Content $File | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#")) {
            if ($line -match '^([^/]+)/([^/]+)$') {
                $list += [PSCustomObject]@{ GOOS = $Matches[1]; GOARCH = $Matches[2] }
            } else {
                Write-Warning "忽略无效平台行: $line"
            }
        }
    }
    if ($list.Count -eq 0) { throw "platforms.txt 中无有效平台" }
    return $list
}

function Invoke-GoBuild {
    param(
        [string]$Root,
        [string]$Goos,
        [string]$Goarch,
        [string]$Package,
        [string]$OutputPath,
        [string]$Ldflags,
        [bool]$CgoEnabled = $false
    )
    $env:GOOS = $Goos
    $env:GOARCH = $Goarch
    if ($CgoEnabled) {
        $env:CGO_ENABLED = "1"
    } else {
        $env:CGO_ENABLED = "0"
    }
    $outDir = Split-Path -Parent $OutputPath
    if ($outDir -and -not (Test-Path $outDir)) {
        New-Item -ItemType Directory -Path $outDir -Force | Out-Null
    }
    & go build -trimpath -ldflags $Ldflags -o $OutputPath $Package
    if ($LASTEXITCODE -ne 0) {
        throw "构建失败: $Goos/$Goarch $Package"
    }
}

# Test-CanBuildWindowsGui 判断当前主机能否构建指定架构的 Fyne GUI（CGO 须同架构或本机 arm64 编 arm64）。
function Test-CanBuildWindowsGui {
    param([string]$Goarch)
    $hostArch = (go env GOHOSTARCH).Trim()
    if ($Goarch -eq $hostArch) { return $true }
    Write-Warning "跳过 windows/$Goarch GUI：Fyne CGO 须本机同架构构建（host=$hostArch）"
    return $false
}

# Invoke-GoBuildGui 构建 Windows 桌面 GUI（Fyne 依赖 CGO，仅 Windows 目标）。
function Invoke-GoBuildGui {
    param(
        [string]$Root,
        [string]$Goarch,
        [string]$OutputPath,
        [string]$Ldflags
    )
    if ($Goarch -ne "amd64" -and $Goarch -ne "arm64") {
        throw "GUI 仅支持 windows/amd64 与 windows/arm64"
    }
    $guiLdflags = "$Ldflags -H windowsgui"
    Write-Host "    gui   windows/$Goarch (Fyne CGO=1)"
    Invoke-GoBuild -Root $Root -Goos "windows" -Goarch $Goarch `
        -Package "./cmd/client-gui" -OutputPath $OutputPath -Ldflags $guiLdflags -CgoEnabled $true
}

function Write-ReleaseManifest {
    param(
        [string]$OutDir,
        [string]$Root,
        [string]$Version,
        [string]$Commit,
        [string]$BuildTime,
        [array]$Artifacts
    )
    $manifest = [ordered]@{
        version   = $Version
        commit    = $Commit
        buildTime = $BuildTime
        goVersion = (go version | Out-String).Trim()
        artifacts = $Artifacts
    }
    $jsonPath = Join-Path $OutDir "manifest.json"
    $manifest | ConvertTo-Json -Depth 5 | Set-Content -Path $jsonPath -Encoding UTF8
    Copy-Item (Join-Path $Root "VERSION") (Join-Path $OutDir "VERSION") -Force
}
