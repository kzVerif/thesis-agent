#requires -Version 5.1
[CmdletBinding()]
param([switch]$Check)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$packageRoot = Split-Path -Parent $PSScriptRoot
$repository = Split-Path -Parent $packageRoot
$outputRoot = Join-Path $packageRoot 'build'
$stage = $null
try {
    if (-not (Test-Path -LiteralPath (Join-Path $repository 'go.mod'))) { throw 'Build requires the source repository. Installation of prebuilt files does not require Go.' }
    if ($null -eq (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go is not installed or is not in PATH.' }
    if ($Check) { Write-Host 'PASS: source and Go are available. No build was started.'; exit 0 }
    Push-Location $repository
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        Write-Host '[1/4] Running regression tests...'
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Tests failed; previous build files were retained.' }
        $stage = Join-Path $outputRoot ('.staging-' + [guid]::NewGuid().ToString('N'))
        [void][IO.Directory]::CreateDirectory($stage)
        Write-Host '[2/4] Building Windows Agent...'
        & go build -trimpath -ldflags '-s -w' -o (Join-Path $stage 'thesis-agent.exe') .
        if ($LASTEXITCODE -ne 0) { throw 'Agent build failed.' }
        Write-Host '[3/4] Building Desktop Helper...'
        & go build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $stage 'thesis-agent-desktop.exe') ./cmd/thesis-agent-desktop
        if ($LASTEXITCODE -ne 0) { throw 'Desktop Helper build failed.' }
        Write-Host '[4/4] Publishing executables and SHA-256 manifest...'
        $artifacts = @(
            @{ Name = 'thesis-agent.exe'; Directory = 'agent' },
            @{ Name = 'thesis-agent-desktop.exe'; Directory = 'desktop-helper' }
        )
        $files = foreach ($artifact in $artifacts) {
            $targetDirectory = Join-Path $outputRoot $artifact.Directory
            [void][IO.Directory]::CreateDirectory($targetDirectory)
            $target = Join-Path $targetDirectory $artifact.Name
            Copy-Item -LiteralPath (Join-Path $stage $artifact.Name) -Destination $target -Force
            @{ path = $artifact.Directory + '/' + $artifact.Name; sha256 = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant(); bytes = (Get-Item -LiteralPath $target).Length }
        }
        $manifest = @{ built_at_utc = [DateTime]::UtcNow.ToString('o'); platform = 'windows/amd64'; go = (& go version); tests = 'go test ./... passed'; files = @($files) }
        $manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $outputRoot 'manifest.json') -Encoding UTF8
        Write-Host "Build complete: $outputRoot"
    } finally { Pop-Location }
    exit 0
} catch {
    Write-Host ('ERROR: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
} finally {
    if ($stage -and (Test-Path -LiteralPath $stage)) {
        $resolvedStage = [IO.Path]::GetFullPath($stage)
        $resolvedOutput = [IO.Path]::GetFullPath($outputRoot)
        if ([IO.Path]::GetDirectoryName($resolvedStage) -ne $resolvedOutput -or [IO.Path]::GetFileName($resolvedStage) -notlike '.staging-*') { throw 'Refusing cleanup outside build staging directory.' }
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force
    }
}
