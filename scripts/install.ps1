# Install voiceloop for the current Windows user.
#
#   powershell -ExecutionPolicy Bypass -File install.ps1 [-HookFrom 192.168.1.20] [-Uninstall]
#
# Put this script next to voiceloop.exe (both come in the release zip) and run it.
# It copies the exe to %LOCALAPPDATA%\VoiceLoop, starts it at logon, adds a Start menu
# entry and starts it. Settings opens on the first run and its About tab shows the
# address and token for the hook.
#
# The firewall rule needs an elevated PowerShell. Without one, the script skips it and
# Windows asks about the firewall the first time voiceloop listens. -HookFrom limits the
# rule to the machine (or machines, comma-separated) that run Claude Code.

param(
  [string]$HookFrom = "",
  [int]$Port = 8765,
  [switch]$Uninstall
)
$ErrorActionPreference = "Stop"

$dir  = Join-Path $env:LOCALAPPDATA "VoiceLoop"
$exe  = Join-Path $dir "voiceloop.exe"
$run  = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$lnk  = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\VoiceLoop.lnk"
$rule = "VoiceLoop-In"
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
  [Security.Principal.WindowsBuiltInRole]::Administrator)

function Stop-VoiceLoop {
  Get-Process voiceloop -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Milliseconds 1500   # Windows can hold the exe for a moment after it exits
}

if ($Uninstall) {
  Stop-VoiceLoop
  Remove-ItemProperty -Path $run -Name VoiceLoop -ErrorAction SilentlyContinue
  Remove-Item $lnk -ErrorAction SilentlyContinue
  if (Get-NetFirewallRule -Name $rule -ErrorAction SilentlyContinue) {
    if ($admin) { Remove-NetFirewallRule -Name $rule; "Removed the firewall rule." }
    else { "The firewall rule $rule is still there. Run this again as administrator to remove it." }
  }
  Remove-Item $exe -ErrorAction SilentlyContinue
  "voiceloop is uninstalled. Your settings, pad profile and icons are still in $dir. Delete that folder to remove them too."
  return
}

$src = Join-Path $PSScriptRoot "voiceloop.exe"
if (-not (Test-Path $src)) { throw "voiceloop.exe isn't next to this script ($PSScriptRoot)." }

New-Item -ItemType Directory -Force $dir | Out-Null
Stop-VoiceLoop
Copy-Item $src $exe -Force
Unblock-File $exe   # downloaded files carry a mark that makes Windows ask before every start

# Start at logon, in the background (tray icon only).
Set-ItemProperty -Path $run -Name VoiceLoop -Value "`"$exe`" --background"

# Start menu entry: starts voiceloop, or opens Settings if it's already running.
$sh = New-Object -ComObject WScript.Shell
$sc = $sh.CreateShortcut($lnk)
$sc.TargetPath = $exe
$sc.WorkingDirectory = $dir
$sc.IconLocation = "$exe,0"
$sc.Description = "voiceloop settings"
$sc.Save()

if ($admin) {
  Remove-NetFirewallRule -Name $rule -ErrorAction SilentlyContinue
  $fw = @{ Name = $rule; DisplayName = "VoiceLoop listener"; Direction = "Inbound"; Action = "Allow";
             Protocol = "TCP"; LocalPort = $Port; Program = $exe; Profile = @("Domain", "Private") }
  if ($HookFrom) { $fw.RemoteAddress = $HookFrom -split "\s*,\s*" }
  New-NetFirewallRule @fw | Out-Null
  if ($HookFrom) { "Firewall: port $Port is open to $HookFrom on private networks." }
  else { "Firewall: port $Port is open on private networks. Use -HookFrom to limit it to the Claude Code machine." }
} else {
  "Skipped the firewall rule (not running as administrator). Windows will ask the first time voiceloop listens: allow it on private networks."
}

Start-Process $exe -WorkingDirectory $dir
"voiceloop $((Get-Item $exe).VersionInfo.ProductVersion) is installed in $dir and starts when you log on."
"Settings is opening. The About tab shows the address and token for the hook."
