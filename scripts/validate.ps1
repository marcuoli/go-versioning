[CmdletBinding()]
param(
    [switch]$BuildOnly,
    [switch]$Race
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

$processSettings = @('GOWORK', 'GOFLAGS', 'CGO_ENABLED')
$previousSettings = @{}
foreach ($settingName in $processSettings) {
    $previousSettings[$settingName] = [Environment]::GetEnvironmentVariable($settingName, 'Process')
}

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $env:GOWORK = 'off'
    $env:GOFLAGS = '-mod=readonly'
    $env:CGO_ENABLED = '0'

    $activeGo = (& go env GOVERSION)
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to determine the installed Go version.'
    }
    if ($activeGo -ne 'go1.27.3') {
        throw ('Go 1.27.3 is required; found {0}.' -f $activeGo)
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
    $output = 'bin/go-versioning'
    if ($targetOS -eq 'windows') {
        $output += '.exe'
    }
    New-Item -ItemType Directory -Path 'bin' -Force | Out-Null
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
