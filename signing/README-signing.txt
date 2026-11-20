CODE SIGNING
============
The exe is not signed, so Windows SmartScreen may say "unknown publisher" the first time.
Nobody can sign it for you without a certificate that belongs to you.

Option A - real certificate (removes the warning for everyone, over time)
  1. Buy an "OV" or "EV" code-signing certificate (for example from DigiCert, Sectigo, SSL.com) or use
     Azure Trusted Signing (cheaper, for individuals/small teams).
  2. Run sign-exe.ps1 with your certificate (see the top of the script).
  EV certificates are trusted by SmartScreen immediately; OV ones build reputation as more people run the file.
  Sign again after every new build.

Option B - self-signed (free, this PC only)
  Run make-self-signed.ps1 as Administrator. It signs the exe and trusts the certificate on this PC.
  Other PCs will still show the warning.

Option C - no certificate: right-click the exe > Properties > tick Unblock, or choose More info > Run anyway.
