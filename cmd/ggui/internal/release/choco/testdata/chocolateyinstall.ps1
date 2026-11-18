# Written by ggui release.
#
# The program is downloaded from the GitHub release and checked against
# its SHA-256 rather than embedded, so this package carries no
# VERIFICATION.txt or LICENSE.txt.
$ErrorActionPreference = 'Stop'
$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition

$packageArgs = @{
  packageName    = $env:ChocolateyPackageName
  unzipLocation  = $toolsDir
  url            = 'https://github.com/alice/hello/releases/download/v1.2.0/hello-world-1.2.0-windows-386.zip'
  checksum       = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
  checksumType   = 'sha256'
  url64bit       = 'https://github.com/alice/hello/releases/download/v1.2.0/hello-world-1.2.0-windows-amd64.zip'
  checksum64     = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
  checksumType64 = 'sha256'
}
Install-ChocolateyZipPackage @packageArgs

# A GUI program: the shim starts it without a console and does not wait.
$exe = Join-Path $toolsDir 'Hello World.exe'
New-Item -ItemType File -Force -Path "$exe.gui" | Out-Null

$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'Hello World.lnk'
Install-ChocolateyShortcut -ShortcutFilePath $shortcut -TargetPath $exe -WorkingDirectory $toolsDir
