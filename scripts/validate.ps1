[CmdletBinding()]
param(
    [switch]$BuildOnly,
    [switch]$Race,
    [ValidateSet('1.27.3', '1.27.4')]
    [string]$GoVersion = '1.27.4',
    [string]$OutputDirectory = $env:GO_VERSIONING_BUILD_DIR
)

$ErrorActionPreference = 'Stop'
if ($BuildOnly -and $Race) {
    throw '-BuildOnly and -Race cannot be combined.'
}

function Invoke-Go {
    param([string[]]$GoArguments)
    & go @GoArguments
    if ($LASTEXITCODE -ne 0) {
        throw ('go {0} failed with exit code {1}.' -f ($GoArguments -join ' '), $LASTEXITCODE)
    }
}

$processSettings = @('GOWORK', 'GOFLAGS', 'CGO_ENABLED', 'GOTOOLCHAIN')
$previousSettings = @{}
foreach ($settingName in $processSettings) {
    $previousSettings[$settingName] = [Environment]::GetEnvironmentVariable($settingName, 'Process')
}

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $env:GOTOOLCHAIN = 'local'
    $env:GOWORK = 'off'
    $env:GOFLAGS = '-mod=readonly'
    $env:CGO_ENABLED = '0'

    $activeGo = (& go env GOVERSION)
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to determine the installed Go version.'
    }
    if ($activeGo -ne ('go' + $GoVersion)) {
        throw ('Go {0} is required; found {1}. Select that compiler explicitly.' -f $GoVersion, $activeGo)
    }

    if (-not $BuildOnly) {
        Write-Host 'Running tests.'
        Invoke-Go -GoArguments @('test', '-count=1', './...')
        Write-Host 'Running go vet.'
        Invoke-Go -GoArguments @('vet', './...')
        Write-Host 'Verifying module checksums.'
        Invoke-Go -GoArguments @('mod', 'verify')
        if ($Race) {
            Write-Host 'Running race tests (requires an existing supported C compiler in CC or PATH).'
            $env:CGO_ENABLED = '1'
            try {
                Invoke-Go -GoArguments @('test', '-race', '-count=1', './...')
            }
            finally {
                $env:CGO_ENABLED = '0'
            }
        }
    }

    $targetOS = (& go env GOOS)
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to determine the build target operating system.'
    }
    if (-not $OutputDirectory) {
        if (Test-Path -LiteralPath 'V:\Development') {
            $OutputDirectory = 'V:\Development\go-versioning\artifacts\build\local'
        }
        elseif ($env:RUNNER_TEMP) {
            $OutputDirectory = Join-Path $env:RUNNER_TEMP 'go-versioning-build'
        }
        else {
            throw 'Set -OutputDirectory or GO_VERSIONING_BUILD_DIR to an external build directory.'
        }
    }
    $output = Join-Path $OutputDirectory 'go-versioning'
    if ($targetOS -eq 'windows') {
        $output += '.exe'
    }
    New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
    Write-Host ('Building local CLI snapshot: {0}' -f $output)
    Invoke-Go -GoArguments @('build', '-trimpath', '-buildvcs=false', '-o', $output, './cmd/go-versioning')
}
finally {
    foreach ($settingName in $processSettings) {
        if ($null -eq $previousSettings[$settingName]) {
            Remove-Item -LiteralPath ('Env:' + $settingName) -ErrorAction SilentlyContinue
        }
        else {
            [Environment]::SetEnvironmentVariable($settingName, $previousSettings[$settingName], 'Process')
        }
    }
    Pop-Location
}
