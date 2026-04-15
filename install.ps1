<#
.SYNOPSIS
    AssistClaw installer for Windows (amd64).

.DESCRIPTION
    Downloads the pre-built binary from GitHub Releases, creates %USERPROFILE%\.assistclaw,
    and verifies the install. Optional -WithMemPalace runs managed MemPalace bootstrap (Python venv + pip).
    Optional -WithGemma runs local-intel GGUF bootstrap.

.PARAMETER Version
    Release tag (e.g. v3.10.4) or 'latest'.

.PARAMETER InstallDir
    Directory for assistclaw.exe (must be on PATH or add manually).

.EXAMPLE
    iwr -useb https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -Version v3.10.4

.EXAMPLE
    .\install.ps1 -WithMemPalace

.EXAMPLE
    .\install.ps1 -WithGemma
#>

param(
    [string]$Version = $(if ($env:ASSISTCLAW_VERSION) { $env:ASSISTCLAW_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:ASSISTCLAW_INSTALL_DIR) { $env:ASSISTCLAW_INSTALL_DIR } else { "" }),
    [switch]$WithMemPalace = $false,
    [switch]$WithGemma = $false
)

$ErrorActionPreference = "Stop"

$Repo = "hridesh-net/AssistClaw"
$ConfigDir = Join-Path $env:USERPROFILE ".assistclaw"

if (-not $InstallDir) {
    $localBin = Join-Path $env:USERPROFILE ".local\bin"
    $InstallDir = $localBin
}

$BinaryName = "assistclaw-windows-amd64.exe"
$FinalName = "assistclaw.exe"

function Write-Info {
    param([string]$Message)
    Write-Host -ForegroundColor Cyan "[assistclaw] $Message"
}

function Write-Ok {
    param([string]$Message)
    Write-Host -ForegroundColor Green "[OK] $Message"
}

function Write-WarnLine {
    param([string]$Message)
    Write-Host -ForegroundColor Yellow "[!] $Message"
}

function Fail-Exit {
    param([string]$Message)
    Write-Host -ForegroundColor Red "[X] $Message"
    exit 1
}

$releaseSegment = if ($Version -eq "latest") { "latest/download" } else { "download/$Version" }
$DownloadUrl = "https://github.com/$Repo/releases/$releaseSegment/$BinaryName"
$DestPath = Join-Path $InstallDir $FinalName

Write-Info "AssistClaw Windows installer"
Write-Info "Install dir: $InstallDir"
Write-Info "Release:     $Version"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

Write-Info "Downloading $BinaryName..."
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("assistclaw-" + [Guid]::NewGuid().ToString("N") + ".exe")
try {
    $params = @{
        Uri             = $DownloadUrl
        OutFile         = $tmp
        UseBasicParsing = $true
        MaximumRetryCount = 3
        RetryIntervalSec  = 2
    }
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        Invoke-WebRequest @params
    } else {
        # Windows PowerShell 5.x: no built-in retries
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $tmp -UseBasicParsing
    }
    Move-Item -Force -Path $tmp -Destination $DestPath
    Write-Ok "Binary installed: $DestPath"
} catch {
    if (Test-Path $tmp) { Remove-Item -Force $tmp -ErrorAction SilentlyContinue }
    Fail-Exit "Download failed: $DownloadUrl — $($_.Exception.Message)"
}

Write-Info "Creating workspace: $ConfigDir"
$subDirs = @(
    (Join-Path $ConfigDir "memory"),
    (Join-Path $ConfigDir "tools"),
    (Join-Path $ConfigDir "logs"),
    (Join-Path $ConfigDir "security"),
    (Join-Path $ConfigDir "skills\bundled"),
    (Join-Path $ConfigDir "skills\custom"),
    (Join-Path $ConfigDir "workspace\public")
)
foreach ($d in $subDirs) {
    if (-not (Test-Path $d)) {
        New-Item -ItemType Directory -Force -Path $d | Out-Null
    }
}
Write-Ok "Workspace ready"

if ($WithMemPalace -or $env:WITH_MEMPALACE -eq "1") {
    Write-Info "Bootstrapping MemPalace (managed venv under $ConfigDir\mempalace) — may download PyPI packages..."
    & $DestPath --log-level info mempalace setup --state-dir $ConfigDir
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "MemPalace bootstrap finished (merge printed YAML into assistclaw.yaml)"
    } else {
        Write-WarnLine "MemPalace bootstrap failed (exit $LASTEXITCODE). Retry: & '$DestPath' mempalace setup --state-dir '$ConfigDir'"
    }
}

if ($WithGemma -or $env:WITH_GEMMA -eq "1") {
    Write-Info "Bootstrapping local-intel GGUF under $ConfigDir\\models\\ (download may be large)..."
    $args = @("--log-level", "info", "local-intel", "setup", "--state-dir", $ConfigDir)
    if ($env:ASSISTCLAW_LOCAL_GEMMA_GGUF_URL) {
        $args += @("--url", $env:ASSISTCLAW_LOCAL_GEMMA_GGUF_URL)
    }
    if ($env:ASSISTCLAW_LOCAL_GEMMA_GGUF_SHA256) {
        $args += @("--sha256", $env:ASSISTCLAW_LOCAL_GEMMA_GGUF_SHA256)
    }
    & $DestPath @args
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "Local-intel GGUF bootstrap finished (merge printed local_intel YAML into assistclaw.yaml)"
    } else {
        Write-WarnLine "Local-intel GGUF bootstrap failed (exit $LASTEXITCODE). Retry: & '$DestPath' local-intel setup --state-dir '$ConfigDir'"
    }
}

Write-Info "Verifying..."
try {
    $verOut = & $DestPath version 2>&1
    Write-Ok "assistclaw $($verOut -join ' ')"
} catch {
    Write-WarnLine "Could not run version check. Add to PATH: $InstallDir"
}

Write-Host ""
Write-Host "  Get started:"
Write-Host "    assistclaw --help"
Write-Host "    assistclaw onboard"
Write-Host "    assistclaw agent --message `"Hello`""
Write-Host ""
Write-Host "  Config: $ConfigDir\assistclaw.yaml"
if ($WithMemPalace -or $env:WITH_MEMPALACE -eq "1") {
    Write-Host "  MemPalace: bootstrapped under $ConfigDir\mempalace\"
} else {
    Write-Host "  MemPalace (optional): .\install.ps1 -WithMemPalace  OR  & '$DestPath' mempalace setup --state-dir '$ConfigDir'"
}
if ($WithGemma -or $env:WITH_GEMMA -eq "1") {
    Write-Host "  Local Intel: GGUF prepared under $ConfigDir\models\ (merge printed local_intel YAML into assistclaw.yaml)"
} else {
    Write-Host "  Local Intel (optional): .\install.ps1 -WithGemma  OR  & '$DestPath' local-intel setup --state-dir '$ConfigDir'"
}
Write-Host ""

$pathDirs = $env:Path -split ';'
if ($pathDirs -notcontains $InstallDir) {
    Write-WarnLine "$InstallDir is not on your PATH."
    Write-Host '  User PATH (PowerShell):'
    Write-Host "    [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$InstallDir', 'User')"
    Write-Host ""
}
