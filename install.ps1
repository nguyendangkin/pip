$repo = "nguyendangkin/pip"
$destDir = "$HOME\.pip"
$destFile = "$destDir\pip.exe"
$url = "https://github.com/$repo/releases/latest/download/pip.exe"

if (!(Test-Path $destDir)) {
    New-Item -ItemType Directory -Path $destDir | Out-Null
}

Write-Host "------------------------------------------" -ForegroundColor Cyan
Write-Host "  Đang cài đặt pip - Secure Archiver      " -ForegroundColor Cyan
Write-Host "------------------------------------------" -ForegroundColor Cyan

try {
    Write-Host "[1/2] Đang tải bản mới nhất từ GitHub..." -NoNewline
    Invoke-WebRequest -Uri $url -OutFile $destFile -ErrorAction Stop
    Write-Host " Xong!" -ForegroundColor Green
} catch {
    Write-Host " Lỗi!" -ForegroundColor Red
    Write-Host "Không thể tải file. Vui lòng kiểm tra lại kết nối mạng hoặc Release v1.0.0 đã có trên GitHub chưa."
    exit
}

Write-Host "[2/2] Cấu hình PATH..." -NoNewline
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -notlike "*$destDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$path;$destDir", "User")
    $env:Path += ";$destDir"
    Write-Host " Đã thêm vào PATH." -ForegroundColor Green
    Write-Host "`nLưu ý: Bạn cần KHỞI ĐỘNG LẠI Terminal (PowerShell/CMD) để sử dụng lệnh 'pip'." -ForegroundColor Yellow
} else {
    Write-Host " PATH đã được cấu hình." -ForegroundColor Gray
}

Write-Host "`nCài đặt hoàn tất! Thử gõ: pip" -ForegroundColor Green
Write-Host "------------------------------------------"
