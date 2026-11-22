#Requires -RunAsAdministrator
<#
  SWISSNAND Backup – Installation auf Windows Server 2016/2019/2022/2025
  Ausführen in PowerShell als Administrator:  .\install.ps1 [-Port 8550]
#>
param(
  [int]$Port = 8550,
  [switch]$Agent,          # als Agent eines zentralen Backup-Servers installieren
  [string]$Server = '',
  [string]$Token = '',
  [string]$Fingerprint = '',
  [string]$Name = ''
)
$ErrorActionPreference = 'Stop'

$dir  = Join-Path $env:ProgramFiles 'SWISSNAND\Backup'
$data = Join-Path $env:ProgramData 'SWISSNAND\Backup'
Write-Host "`n  SWISSNAND Backup wird installiert ...`n" -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path $dir | Out-Null
Copy-Item -Force (Join-Path $PSScriptRoot 'snbackup.exe') $dir
Copy-Item -Force (Join-Path $PSScriptRoot 'LICENSE') $dir -ErrorAction SilentlyContinue
Copy-Item -Force (Join-Path $PSScriptRoot 'uninstall.ps1') $dir -ErrorAction SilentlyContinue

# Programmordner zum System-PATH hinzufügen (für die Kommandozeile)
$path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
if ($path -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable('Path', "$path;$dir", 'Machine')
  $env:Path += ";$dir"
}

if ($Agent) {
  if (-not $Server -or -not $Token) { throw 'Für -Agent bitte -Server und -Token angeben (aus der Web-Konsole, Seite Agenten).' }
  $exe = Join-Path $dir 'snbackup.exe'
  $enrollArgs = @('agent', 'enroll', '--server', $Server, '--token', $Token)
  if ($Fingerprint) { $enrollArgs += @('--fingerprint', $Fingerprint) }
  if ($Name) { $enrollArgs += @('--name', $Name) }
  & $exe @enrollArgs
  if ($LASTEXITCODE -ne 0) { throw 'Registrierung fehlgeschlagen' }
  & $exe service install --agent
  Write-Host "`n  Agent installiert. Er erscheint in der Web-Konsole unter «Agenten».`n" -ForegroundColor Green
  return
}

# Dienst (SYSTEM, Autostart, automatischer Neustart) + Firewall-Regel
& (Join-Path $dir 'snbackup.exe') service install --data "$data" --listen ":$Port"

# Startmenü-Verknüpfung zur Web-Konsole
$lnk = Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\SWISSNAND Backup.url'
"[InternetShortcut]`r`nURL=https://localhost:$Port/" | Set-Content -Encoding ASCII $lnk

Start-Sleep -Seconds 4
$pwFile = Join-Path $data 'initial-admin-password'
Write-Host "`n  Installation abgeschlossen." -ForegroundColor Green
Write-Host "  Web-Konsole:  https://localhost:$Port"
if (Test-Path $pwFile) { Write-Host "  Benutzer admin, Startpasswort: $(Get-Content $pwFile)" -ForegroundColor Yellow }
Write-Host "  Kommandozeile: snbackup help (neues Terminal öffnen)`n"
Start-Process "https://localhost:$Port/"
