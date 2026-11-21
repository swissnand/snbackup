#Requires -RunAsAdministrator
# Entfernt den Dienst und das Programm. Konfiguration und Backups bleiben erhalten.
$dir = Join-Path $env:ProgramFiles 'SWISSNAND\Backup'
& (Join-Path $dir 'snbackup.exe') service uninstall
Remove-NetFirewallRule -DisplayName 'SWISSNAND Backup Web-Konsole' -ErrorAction SilentlyContinue
Remove-Item (Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\SWISSNAND Backup.url') -ErrorAction SilentlyContinue
$path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
[Environment]::SetEnvironmentVariable('Path', (($path -split ';') -ne $dir -join ';'), 'Machine')
Write-Host "Deinstalliert. Daten in $env:ProgramData\SWISSNAND\Backup bleiben erhalten." -ForegroundColor Green
Start-Sleep 1
Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
