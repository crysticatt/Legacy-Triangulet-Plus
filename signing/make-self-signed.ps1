# Makes a SELF-SIGNED certificate, signs the exe with it and trusts it on THIS PC only.
# Effect: on this PC the exe shows a publisher name and no "unknown publisher" prompt.
# It does NOT remove SmartScreen warnings on other people's PCs - that needs a real certificate
# (see signing\README-signing.txt). Run PowerShell as Administrator.
param([string]$Exe = "$PSScriptRoot\..\TrianguletOpener.exe", [string]$Name = "Triangulet Opener")
$ErrorActionPreference = "Stop"
$cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject "CN=$Name" -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddYears(3)
foreach ($store in "Root","TrustedPublisher") {
  $s = New-Object System.Security.Cryptography.X509Certificates.X509Store($store, "LocalMachine")
  $s.Open("ReadWrite"); $s.Add($cert); $s.Close()
}
Set-AuthenticodeSignature -FilePath $Exe -Certificate $cert -HashAlgorithm SHA256 -TimestampServer "http://timestamp.digicert.com" | Format-List
