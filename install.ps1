<#
.SYNOPSIS
    Installation script for grepai on Windows.
.DESCRIPTION
    Downloads the latest version of grepai from GitHub, installs it to
    %LOCALAPPDATA%\Programs\grepai and updates the user PATH.
#>

$repo = "yoanbernabeu/grepai"
$installDir = "$env:LOCALAPPDATA\Programs\grepai"
$binName = "grepai.exe"

Write-Host "--- grepai Installer for Windows ---" -ForegroundColor Cyan

# 1. Get latest version from GitHub API
try {
    $releaseUrl = "https://api.github.com/repos/$repo/releases/latest"
    $latestRelease = Invoke-RestMethod -Uri $releaseUrl
    $versionTag = $latestRelease.tag_name
    $versionNumber = $versionTag.TrimStart('v')
    Write-Host "Detected version: $versionTag" -ForegroundColor Gray
} catch {
    Write-Error "Error: Could not connect to GitHub API."
    exit 1
}

# 2. Identify the correct asset (grepai_{VERSION}_windows_amd64.zip)
$assetName = "grepai_$($versionNumber)_windows_amd64.zip"
$asset = $latestRelease.assets | Where-Object { $_.name -eq $assetName }

if (-not $asset) {
    Write-Error "Package $assetName for Windows was not found in the current release."
    exit 1
}

$downloadUrl = $asset.browser_download_url

# 3. Prepare installation directories
if (!(Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

$tempZip = "$env:TEMP\grepai_install.zip"

# 4. Download and Extraction
Write-Host "Downloading $assetName..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip

Write-Host "Extracting files to $installDir..." -ForegroundColor Cyan
# Extract content. Force overwrites if it already exists.
Expand-Archive -Path $tempZip -DestinationPath $installDir -Force

# Clean up temporary zip
Remove-Item $tempZip

# 5. Configure User PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    Write-Host "Adding grepai to User PATH..." -ForegroundColor Yellow
    $newPath = "$userPath;$installDir"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

    # Update current session so 'grepai' works immediately
    $env:Path += ";$installDir"
}

Write-Host "`nInstallation completed successfully!" -ForegroundColor Green
Write-Host "Installation location: $installDir" -ForegroundColor Gray
Write-Host "Run 'grepai version' to confirm." -ForegroundColor White
