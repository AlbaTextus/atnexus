param(
    [Parameter(Position = 0)]
    [ValidatePattern('^[a-zA-Z0-9][a-zA-Z0-9._-]*$')]
    [string]$Name
)

$ErrorActionPreference = 'Stop'
$buildArguments = @(Join-Path $PSScriptRoot 'build-linux-amd64.mjs')
if ($Name) {
    $buildArguments += $Name
}
& node @buildArguments
if ($LASTEXITCODE -ne 0) {
    throw "Linux AMD64 build failed (exit code $LASTEXITCODE). See the build output above."
}
