# Shared by the uninstallers. Callers supply a fixed known-folder parent/name.
function Assert-CleanupTree([string]$Path, [string]$Parent, [string]$Name) {
    if (-not [IO.Path]::IsPathRooted($Parent) -or $Name -notin @('ThesisAgentDev', 'ThesisAgentDesktop')) {
        throw 'Invalid uninstall directory boundary.'
    }
    $expected = [IO.Path]::GetFullPath((Join-Path $Parent $Name))
    $target = [IO.Path]::GetFullPath($Path)
    if ($target -ine $expected) { throw "Refusing cleanup outside the installed directory: $target" }
    $ancestor = $target
    while ($ancestor) {
        if (Test-Path -LiteralPath $ancestor) {
            if (((Get-Item -LiteralPath $ancestor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Refusing linked uninstall path: $ancestor"
            }
        }
        $directory = [IO.Directory]::GetParent($ancestor)
        if ($null -eq $directory) { break }
        $ancestor = $directory.FullName
    }
    if (-not (Test-Path -LiteralPath $target)) { return }
    if (-not (Test-Path -LiteralPath $target -PathType Container)) { throw "Expected an installation directory: $target" }
    $pending = New-Object 'Collections.Generic.Queue[string]'
    $pending.Enqueue($target)
    while ($pending.Count -gt 0) {
        foreach ($item in Get-ChildItem -LiteralPath $pending.Dequeue() -Force -ErrorAction Stop) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Refusing linked uninstall entry: $($item.FullName)"
            }
            if ($item.PSIsContainer) { $pending.Enqueue($item.FullName) }
        }
    }
}

function Remove-InstalledTree([string]$Path, [string]$Parent, [string]$Name) {
    Assert-CleanupTree $Path $Parent $Name
    $target = [IO.Path]::GetFullPath($Path)
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Recurse -Force -ErrorAction Stop
    }
    if (Test-Path -LiteralPath $target) { throw "Uninstall directory still exists: $target" }
}
