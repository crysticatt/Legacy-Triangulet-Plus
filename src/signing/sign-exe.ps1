# Signs TrianguletOpener.exe with YOUR code-signing certificate.
# Needs the Windows SDK's signtool.exe (installed with Visual Studio Build Tools / Windows SDK).
#
# Usage (PowerShell):
#   .\sign-exe.ps1 -Pfx C:\path\to\certificate.pfx -Password "pfx password"
#   .\sign-exe.ps1 -Thumbprint 0123ABC...      # a certificate already in your Windows store
param(
  [string]$Exe = "$PSScriptRoot\..\TrianguletOpener.exe",
  [string]$Pfx,
  [string]$Password,
  [string]$Thumbprint,
  [string]$Timestamp = "http://timestamp.digicert.com"
)
$ErrorActionPreference = "Stop"
$signtool = (Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | Select-Object -First 1).FullName
if (-not $signtool) { throw "signtool.exe not found. Install the Windows SDK (Signing Tools)." }
if ($Pfx)            { & $signtool sign /fd SHA256 /tr $Timestamp /td SHA256 /f $Pfx /p $Password $Exe }
elseif ($Thumbprint) { & $signtool sign /fd SHA256 /tr $Timestamp /td SHA256 /sha1 $Thumbprint $Exe }
else { throw "Give -Pfx and -Password, or -Thumbprint." }
& $signtool verify /pa /v $Exe
Write-Host "Signed: $Exe"
