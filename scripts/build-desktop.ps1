$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$goCommand = if ($env:GO) { $env:GO } else { 'go' }
$version = if ($env:VERSION) { $env:VERSION } else { 'dev' }
Push-Location frontend
try {
    if ($env:AISSH_SKIP_INSTALL -ne '1') {
        npm ci --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally { Pop-Location }
New-Item -ItemType Directory -Force dist/desktop | Out-Null
& $goCommand build -trimpath -ldflags "-s -w -X main.version=$version" -o dist/desktop/aissh.exe ./cmd/aissh
if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
& $goCommand build -tags desktop,production -trimpath -ldflags '-s -w -H windowsgui' -o dist/desktop/aissh-desktop.exe ./cmd/aissh-desktop
if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
Write-Host 'Built: dist/desktop/aissh-desktop.exe (keep aissh.exe alongside it)'
