# Installs the latest substack-mcp release for Windows.
#
#   irm https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.ps1 | iex
#
# Set $env:INSTALL_DIR to choose the folder and $env:VERSION to pin a release.
$ErrorActionPreference = 'Stop'

$repo = 'gabriellsalesx/substack-mcp'
$dir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\substack-mcp' }
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$base = if ($env:VERSION) { "https://github.com/$repo/releases/download/$($env:VERSION)" } else { "https://github.com/$repo/releases/latest/download" }
$archive = "substack-mcp_windows_$arch.zip"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading $archive..."
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

    $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($archive))$" }
    $expected = ($line -split ' ')[0]
    $actual = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
    if (-not $expected -or $expected -ne $actual) { throw "checksum mismatch for $archive" }

    Expand-Archive (Join-Path $tmp $archive) -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    Copy-Item (Join-Path $tmp 'substack-mcp.exe') (Join-Path $dir 'substack-mcp.exe') -Force
} finally {
    Remove-Item $tmp -Recurse -Force
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
    Write-Host "Added $dir to your PATH (open a new terminal to use it)."
}
Write-Host "Installed $(& (Join-Path $dir 'substack-mcp.exe') version) to $dir"
Write-Host 'Next: substack-mcp login'
