Param(
  [string]$Version,
  [string]$Repo = "cloudcarver/botx",
  [string]$InstallDir = "${env:LOCALAPPDATA}\botx\bin",
  [switch]$SkipPath
)

$ErrorActionPreference = "Stop"

if (-not $Version) {
  $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
  $Version = $latest.tag_name
}

if (-not $Version) {
  Write-Error "Could not determine version"
  exit 1
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "ARM64" { "arm64" }
  "AMD64" { "amd64" }
  "x86" { "amd64" }
  Default { "amd64" }
}

$asset = "botx_${Version}_windows_${arch}.zip"
$url = "https://github.com/$Repo/releases/download/$Version/$asset"

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "botx-$Version"
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
$zipPath = Join-Path $tempDir $asset

Invoke-WebRequest -Uri $url -OutFile $zipPath
Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Path (Join-Path $tempDir "botx.exe") -Destination (Join-Path $InstallDir "botx.exe") -Force

if (-not $SkipPath) {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (-not $userPath) {
    $userPath = ""
  }
  $parts = $userPath -split ";" | Where-Object { $_ -ne "" }
  if ($parts -notcontains $InstallDir) {
    $newPath = ($parts + $InstallDir) -join ";"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Added $InstallDir to user PATH. Open a new terminal to use botx."
  }
}

Write-Host "Installed botx $Version to $InstallDir"
Write-Host "Run: botx --version"
