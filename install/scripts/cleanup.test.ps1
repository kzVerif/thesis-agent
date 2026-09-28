# Run with powershell.exe -NoProfile -File install/scripts/cleanup.test.ps1.
# Exercises only fresh temporary fixtures, never installed Agent directories.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'cleanup.ps1')
$testParent = Join-Path ([IO.Path]::GetTempPath()) ('agent-cleanup-test-' + [guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testParent)
try {
    $target = Join-Path $testParent 'ThesisAgentDev'
    [void][IO.Directory]::CreateDirectory((Join-Path $target 'data\downloads'))
    foreach ($name in @('.env', 'agent_config.json', 'enrollment_state.json', 'data\downloads\sample.txt')) {
        [IO.File]::WriteAllText((Join-Path $target $name), 'test fixture')
    }
    (Get-Item -LiteralPath (Join-Path $target '.env') -Force).Attributes = [IO.FileAttributes]::Hidden
    $sentinel = Join-Path $testParent 'keep.txt'
    [IO.File]::WriteAllText($sentinel, 'outside target')
    foreach ($bad in @($testParent, (Join-Path $testParent 'Other'), (Join-Path $target '..'))) {
        $rejected = $false
        try { Remove-InstalledTree $bad $testParent 'ThesisAgentDev' } catch { $rejected = $true }
        if (-not $rejected) { throw "Unsafe path was accepted: $bad" }
    }
    Remove-InstalledTree $target $testParent 'ThesisAgentDev'
    if (Test-Path -LiteralPath $target) { throw 'Runtime files remain.' }
    if (-not (Test-Path -LiteralPath $sentinel)) { throw 'Outside file was removed.' }
    Remove-InstalledTree $target $testParent 'ThesisAgentDev'
    $helper = Join-Path $testParent 'ThesisAgentDesktop'
    [void][IO.Directory]::CreateDirectory($helper)
    [IO.File]::WriteAllText((Join-Path $helper 'helper.exe'), 'test fixture')
    Remove-InstalledTree $helper $testParent 'ThesisAgentDesktop'
    Write-Output 'PASS: runtime/hidden files and helper files removed; outside paths rejected; repeat removal succeeds.'
} finally {
    $resolved = [IO.Path]::GetFullPath($testParent)
    $tempParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
    if ([IO.Path]::GetDirectoryName($resolved) -ine $tempParent -or [IO.Path]::GetFileName($resolved) -notlike 'agent-cleanup-test-*') {
        throw 'Refusing cleanup outside the temporary test directory.'
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
