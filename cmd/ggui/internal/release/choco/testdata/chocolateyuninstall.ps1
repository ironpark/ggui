# Written by ggui release.
$ErrorActionPreference = 'Stop'

$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'Hello World.lnk'
if (Test-Path $shortcut) {
  Remove-Item $shortcut -Force
}
