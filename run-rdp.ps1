# VS Code terminal (PowerShell) - paste & run
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

New-Item -ItemType Directory -Force -Path bin, results, configs, wordlists | Out-Null

Write-Host "Building..." -ForegroundColor Cyan
go build -o bin/checker.exe .
go build -o bin/demo-server.exe ./demo-server

Write-Host ""
Write-Host "=== Terminal 1: Demo Server ===" -ForegroundColor Green
Write-Host "  .\bin\demo-server.exe"
Write-Host ""
Write-Host "=== Terminal 2: Dashboard ===" -ForegroundColor Green
Write-Host "  .\bin\checker.exe"
Write-Host ""
Write-Host "Browser: http://localhost:8080" -ForegroundColor Yellow
