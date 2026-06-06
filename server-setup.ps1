# Server par pehli baar (PowerShell) - git pull ke baad
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

# Docker installed ho to:
docker compose up -d --build

Write-Host "Dashboard: http://YOUR_SERVER_IP:8080" -ForegroundColor Green
Write-Host "FlareSolverr: http://YOUR_SERVER_IP:8191" -ForegroundColor Green
