$repo = "nguyendangkin/chin"
$destDir = "$HOME\.chin"
$destFile = "$destDir\chin.exe"
$url = "https://github.com/$repo/releases/latest/download/chin.exe"

if (!(Test-Path $destDir)) {
    New-Item -ItemType Directory -Path $destDir | Out-Null
}

Write-Host "------------------------------------------" -ForegroundColor Cyan
Write-Host "  Installing chin - Secure Archiver        " -ForegroundColor Cyan
Write-Host "------------------------------------------" -ForegroundColor Cyan

try {
    Write-Host "[1/2] Downloading latest version from GitHub..." -NoNewline
    Invoke-WebRequest -Uri $url -OutFile $destFile -ErrorAction Stop
    Write-Host " Done!" -ForegroundColor Green
} catch {
    Write-Host " Error!" -ForegroundColor Red
    Write-Host "Could not download file. Please check your internet connection or if the release exists on GitHub."
    exit
}

Write-Host "[2/2] Configuring PATH..." -NoNewline
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -notlike "*$destDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$path;$destDir", "User")
    $env:Path += ";$destDir"
    Write-Host " Added to PATH." -ForegroundColor Green
    Write-Host "`nNOTE: Please RESTART your Terminal (PowerShell/CMD) to use the 'chin' command." -ForegroundColor Yellow
} else {
    Write-Host " PATH is already configured." -ForegroundColor Gray
}

Write-Host "`nInstallation complete! Try typing: chin" -ForegroundColor Green
Write-Host "------------------------------------------"
