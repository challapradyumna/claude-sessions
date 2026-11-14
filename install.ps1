# Install claude-sessions on Windows (PowerShell 5.1+):
#   irm https://raw.githubusercontent.com/challapradyumna/claude-sessions/main/install.ps1 | iex
# Options (environment variables):
#   CLAUDE_SESSIONS_INSTALL_DIR   where to put the binary (default %LOCALAPPDATA%\Programs\claude-sessions)
#   CLAUDE_SESSIONS_VERSION       a release tag such as v0.1.0 (default: latest)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$repo = 'challapradyumna/claude-sessions'
$dir = if ($env:CLAUDE_SESSIONS_INSTALL_DIR) { $env:CLAUDE_SESSIONS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\claude-sessions' }
$version = if ($env:CLAUDE_SESSIONS_VERSION) { $env:CLAUDE_SESSIONS_VERSION } else { 'latest' }
$base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
if ($env:CLAUDE_SESSIONS_BASE_URL) { $base = $env:CLAUDE_SESSIONS_BASE_URL }  # for testing against a local build

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { throw "Unsupported CPU: $env:PROCESSOR_ARCHITECTURE" }
}
$asset = "claude-sessions_windows_$arch.exe"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading $asset ($version)..."
  Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
  Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { ($_ -split '\s+')[1] -replace '^\*', '' -eq $asset }
  $expected = if ($line) { ($line -split '\s+')[0] } else { '' }
  $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLower()
  if (-not $expected -or $expected -ne $actual) { throw "Checksum mismatch for $asset (expected '$expected', got '$actual')." }

  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $exe = Join-Path $dir 'claude-sessions.exe'
  Move-Item -Force (Join-Path $tmp $asset) $exe
  Write-Host "Installed $(& $exe version) to $exe"
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
  Write-Host "Added $dir to your user PATH. Open a new terminal to use 'claude-sessions'."
}
