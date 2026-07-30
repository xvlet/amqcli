$ErrorActionPreference = "Stop"

$BIN = "amqcli"
$REPO = "xvlet/amqcli"

# Define install directory
$InstallDir = if ($env:AMQCLI_INSTALL_DIR) { $env:AMQCLI_INSTALL_DIR } else { "$env:USERPROFILE\.local\bin" }

Write-Host ""
Write-Host "                                _ _ "
Write-Host "   __ _ _ __ ___   __ _  ___| (_) "
Write-Host "  / _\` | '_ \` _ \ / _\` |/ __| | | "
Write-Host " | (_| | | | | | | (_| | (__| | | "
Write-Host "  \__,_|_| |_| |_|\__, |\___|_|_| "
Write-Host "                  |___/           amqcli installer 💫"
Write-Host "                                  github.com/$REPO"
Write-Host ""

# Detect Architecture
$Arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'AMD64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'AMD64') { "amd64" } else { "arm64" }
$Target = "windows_$Arch"

Write-Host "  > detected windows/$Arch" -ForegroundColor Green

# Fetch latest release from GitHub API
Write-Host "  > fetching latest release manifest..." -ForegroundColor Green
$ManifestUrl = "https://api.github.com/repos/$REPO/releases/latest"
try {
    $Manifest = Invoke-RestMethod -Uri $ManifestUrl -UseBasicParsing
} catch {
    Write-Host "  X can't reach GitHub API. Please try again later." -ForegroundColor Red
    exit 1
}

$Version = $Manifest.tag_name

# Find the matching asset (looks for .zip or .tar.gz)
$Asset = $Manifest.assets | Where-Object { $_.name -match $Target -and ($_.name -match '\.zip$' -or $_.name -match '\.tar\.gz$') } | Select-Object -First 1

if (-not $Asset) {
    Write-Host "  X release manifest does not include a binary for $Target" -ForegroundColor Red
    exit 1
}

$DownloadUrl = $Asset.browser_download_url
$FileName = $Asset.name

Write-Host "  > downloading $Version..." -ForegroundColor Green
$TempDir = Join-Path $env:TEMP "amqcli_install_$([guid]::NewGuid().ToString().Substring(0,8))"
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
$DownloadPath = Join-Path $TempDir $FileName

Invoke-WebRequest -Uri $DownloadUrl -OutFile $DownloadPath -UseBasicParsing

Write-Host "  > extracting..." -ForegroundColor Green
$OriginalLocation = Get-Location
if ($FileName.EndsWith(".zip")) {
    Expand-Archive -Path $DownloadPath -DestinationPath $TempDir -Force
} else {
    Set-Location $TempDir
    tar -xzf $FileName
}

$BinPath = Get-ChildItem -Path $TempDir -Recurse -Filter "$BIN.exe" | Select-Object -First 1

if (-not $BinPath) {
    # Sometimes windows binaries don't have .exe in some weird releases, check without extension just in case
    $BinPath = Get-ChildItem -Path $TempDir -Recurse -Filter "$BIN" | Select-Object -First 1
    if (-not $BinPath) {
        Write-Host "  X could not find $BIN.exe in the extracted archive" -ForegroundColor Red
        Set-Location $OriginalLocation
        Remove-Item -Recurse -Force $TempDir
        exit 1
    }
}

# Install
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$DestPath = Join-Path $InstallDir "$BIN.exe"
Move-Item -Path $BinPath.FullName -Destination $DestPath -Force

# Install config file if it doesn't exist
$ConfigDest = Join-Path $env:USERPROFILE ".amqcli.yml"
$ConfigSrc = Join-Path $TempDir "config.yml"
if (-not (Test-Path $ConfigDest) -and (Test-Path $ConfigSrc)) {
    Copy-Item -Path $ConfigSrc -Destination $ConfigDest
    Write-Host "  > installed default config to $ConfigDest" -ForegroundColor Green
}

Write-Host "  > installed $BIN to $DestPath" -ForegroundColor Green

# Cleanup
Set-Location $OriginalLocation
Remove-Item -Recurse -Force $TempDir

# Check PATH
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$SysPath = [Environment]::GetEnvironmentVariable("PATH", "Machine")

if (($UserPath -notmatch [regex]::Escape($InstallDir)) -and ($SysPath -notmatch [regex]::Escape($InstallDir))) {
    Write-Host ""
    Write-Host "  > adding $InstallDir to your PATH..." -ForegroundColor Green
    $NewPath = if ([string]::IsNullOrWhiteSpace($UserPath)) { $InstallDir } else { "$UserPath;$InstallDir" }
    [Environment]::SetEnvironmentVariable('PATH', $NewPath, 'User')
    Write-Host "  ! Please restart your terminal to apply the new PATH." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "  > ready. run '$BIN' to get started." -ForegroundColor Green
Write-Host ""
