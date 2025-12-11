$ErrorActionPreference = "Stop"

$Owner = "endrilickollari"
$Repo = "debtdrone-cli"
$BinaryName = "debtdrone.exe"

Write-Host "🔍 Looking for latest release..."

try {
    $ReleaseUrl = "https://api.github.com/repos/$Owner/$Repo/releases/latest"
    $ReleaseData = Invoke-RestMethod -Uri $ReleaseUrl -Method Get
} catch {
    Write-Error "Failed to fetch release data. Check your internet connection."
    exit 1
}

$Asset = $ReleaseData.assets | Where-Object { $_.name -like "*_Windows_x86_64.zip" } | Select-Object -First 1

if ($null -eq $Asset) {
    Write-Error "❌ Could not find a Windows x86_64 release asset."
    exit 1
}

$DownloadUrl = $Asset.browser_download_url
Write-Host "⬇️  Downloading from: $DownloadUrl"

$TempDir = [System.IO.Path]::GetTempPath()
$ZipPath = Join-Path $TempDir "debtdrone.zip"
$ExtractDir = Join-Path $TempDir "debtdrone_extract"

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath
    
    if (Test-Path $ExtractDir) {
        Remove-Item -Path $ExtractDir -Recurse -Force
    }
    
    Expand-Archive -Path $ZipPath -DestinationPath $ExtractDir -Force
} catch {
    Write-Error "Failed to download or extract the archive."
    exit 1
}

$InstallDir = Join-Path $env:LOCALAPPDATA "debtdrone\bin"
if (-not (Test-Path $InstallDir)) {
    New-Item -Path $InstallDir -ItemType Directory -Force | Out-Null
}

$SourceBinary = Join-Path $ExtractDir $BinaryName
$DestBinary = Join-Path $InstallDir $BinaryName

Write-Host "🚀 Installing to $InstallDir..."

if (Test-Path $SourceBinary) {
    Copy-Item -Path $SourceBinary -Destination $DestBinary -Force
} else {
    $FoundBinary = Get-ChildItem -Path $ExtractDir -Filter $BinaryName -Recurse | Select-Object -First 1
    if ($FoundBinary) {
         Copy-Item -Path $FoundBinary.FullName -Destination $DestBinary -Force
    } else {
        Write-Error "Could not find $BinaryName in the downloaded archive."
        exit 1
    }
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($UserPath -split ';' | Where-Object { $_ -eq $InstallDir })) {
    Write-Host "🔧 Adding to PATH..."
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "⚠️  You may need to restart your terminal for PATH changes to take effect."
}

Write-Host "✅ Installation complete! Run 'debtdrone --help' to start."
