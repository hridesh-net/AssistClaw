<#
.SYNOPSIS
AssistClaw Installer for Windows

.DESCRIPTION
Downloads the pre-compiled AssistClaw binary from GitHub Releases,
sets up the $HOME\.assistclaw workspace, and optionally adds it to the user's PATH.

.EXAMPLE
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/hridesh-net/AssistClaw/main/install.ps1" -OutFile "install.ps1"; .\install.ps1
#>

$ErrorActionPreference = "Stop"

# Configuration
$BinaryHost = "https://github.com/hridesh-net/AssistClaw/releases/latest/download"
$InstallDir = Join-Path $env:USERPROFILE "AppData\Local\Microsoft\WindowsApps" # Default path usually in PATH
$ConfigDir  = Join-Path $env:USERPROFILE ".assistclaw"
$BinaryName = "assistclaw-windows-amd64.exe"
$FinalName  = "assistclaw.exe"

function Write-Log {
    param([string]$Message)
    Write-Host -ForegroundColor Cyan "[assistclaw] $Message"
}

function Write-Ok {
    param([string]$Message)
    Write-Host -ForegroundColor Green "[✓] $Message"
}

function Write-Warn {
    param([string]$Message)
    Write-Host -ForegroundColor Yellow "[!] $Message"
}

function Write-Error {
    param([string]$Message)
    Write-Host -ForegroundColor Red "[✗] $Message"
    Exit 1
}

# 1. Download Binary
Write-Log "Downloading pre-built Windows binary..."
$DownloadUrl = "$BinaryHost/$BinaryName"
$DestPath = Join-Path $InstallDir $FinalName

# Ensure directory exists
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $DestPath -UseBasicParsing
    Write-Ok "Binary installed to: $DestPath"
} catch {
    Write-Error "Failed to download binary from $DownloadUrl. Error: $_"
}

# 2. Setup Config Directory
Write-Log "Setting up $ConfigDir config directory..."
$SubDirs = @("memory", "tools", "skills", "logs")
foreach ($dir in $SubDirs) {
    $path = Join-Path $ConfigDir $dir
    if (-not (Test-Path $path)) {
        New-Item -ItemType Directory -Force -Path $path | Out-Null
    }
}
Write-Ok "Workspace folders created"



# 4. Verify
Write-Log "Verifying installation..."
try {
    $Output = & $DestPath version 2>&1
    Write-Ok "AssistClaw installed successfully!"
    Write-Host ""
    Write-Host "  Get started:"
    Write-Host "    assistclaw --help"
    Write-Host "    assistclaw providers list"
    Write-Host "    assistclaw agent --message `"Hello!`""
    Write-Host ""
} catch {
    Write-Warn "Binary downloaded, but could not execute. Ensure $InstallDir is in your system PATH."
}
