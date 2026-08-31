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

# Get-VersionQuad 将 VERSION（可含 -dev）转为 Windows 资源用的 a.b.c.d。
function Get-VersionQuad {
    param([string]$Version)
    $core = ($Version -split '-', 2)[0]
    $parts = @($core.Split('.') | Where-Object { $_ -ne '' })
    while ($parts.Count -lt 4) { $parts += '0' }
    if ($parts.Count -gt 4) { $parts = $parts[0..3] }
    return ($parts -join '.')
}

# Sync-FyneAppTomlFromVersion 把 cmd/client-gui/FyneApp.toml 的 Version 写成根目录 VERSION（禁止手写死版本）。
function Sync-FyneAppTomlFromVersion {
    param(
        [string]$Root,
        [string]$Version = ""
    )
    if (-not $Version) {
        $Version = Get-ProjectVersion -Root $Root
    }
    $path = Join-Path $Root "cmd/client-gui/FyneApp.toml"
    if (-not (Test-Path $path)) {
        Write-Warning "未找到 FyneApp.toml，跳过版本同步: $path"
        return
    }
    $text = Get-Content $path -Raw -Encoding UTF8
    if ($text -notmatch '(?m)^Version\s*=') {
        throw "FyneApp.toml 缺少 Version 字段: $path"
    }
    $text = [regex]::Replace($text, '(?m)^Version\s*=\s*".*"\s*$', "Version = `"$Version`"")
    # 保持原有换行风格，仅确保末尾一个换行
    $nl = if ($text -match "`r`n") { "`r`n" } else { "`n" }
    $text = $text.TrimEnd("`r", "`n") + $nl
    [System.IO.File]::WriteAllText($path, $text)
    Write-Host "    FyneApp.toml Version=$Version"
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
        [bool]$CgoEnabled = $false,
        # Tags 可选构建标签（空格分隔）；GUI 须含 migrated_fynedo，否则 Fyne 仍打「not migrated」警告。
        [string]$Tags = ""
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
    $buildArgs = @("build", "-trimpath", "-ldflags", $Ldflags)
    if ($Tags -and $Tags.Trim() -ne "") {
        $buildArgs += @("-tags", $Tags.Trim())
    }
    $buildArgs += @("-o", $OutputPath, $Package)
    & go @buildArgs
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
    # migrated_fynedo：与 FyneApp.toml [Migrations] fyneDo 等价；纯 go build 不读 toml，必须带此 tag。
    Write-Host "    gui   windows/$Goarch (Fyne CGO=1 tags=migrated_fynedo)"
    Invoke-GoBuild -Root $Root -Goos "windows" -Goarch $Goarch `
        -Package "./cmd/client-gui" -OutputPath $OutputPath -Ldflags $guiLdflags -CgoEnabled $true `
        -Tags "migrated_fynedo"
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
