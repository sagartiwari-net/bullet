# Run in PowerShell (Administrator) - installs Go when winget is not available
$ErrorActionPreference = "Stop"

$goVersion = "1.24.2"
$msi = "go$goVersion.windows-amd64.msi"
$url = "https://go.dev/dl/$msi"

Write-Host "Downloading Go $goVersion..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $url -OutFile $msi

Write-Host "Installing Go (silent)..." -ForegroundColor Cyan
Start-Process msiexec.exe -Wait -ArgumentList "/i $msi /quiet"

Remove-Item $msi -Force

# Refresh PATH in current session
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")

Write-Host ""
& "C:\Program Files\Go\bin\go.exe" version
Write-Host ""
Write-Host "Go installed! Close and reopen VS Code terminal, then run:" -ForegroundColor Green
Write-Host '  go build -o bin\checker.exe .'
Write-Host '  go build -o bin\demo-server.exe .\demo-server'
