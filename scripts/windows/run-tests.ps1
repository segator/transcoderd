param(
    [Parameter(Mandatory=$true)][string]$TestDirectory,
    [string]$Bundle,
    [string]$Fixture
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
# Also normalize the environment when launched through WSL interop.
$env:PATH = "$env:SystemRoot\System32;$env:SystemRoot;" + $env:PATH
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$failed = $false
$tests = @(Get-ChildItem -LiteralPath $TestDirectory -Filter '*.test.exe')
if ($tests.Count -eq 0) { throw 'No Windows test executables found.' }
if ($Bundle -or $Fixture) {
    if (-not $Bundle -or -not $Fixture) { throw 'Bundle and Fixture must be supplied together.' }
    if (-not (Test-Path (Join-Path $TestDirectory 'windows-bundle.test.exe'))) {
        throw 'The bundled media/OCR test executable is missing.'
    }
}
foreach ($test in $tests) {
    $arguments = @('-test.v','-test.timeout=120s')
    if ($test.Name -eq 'windows-bundle.test.exe') {
        if (-not $Bundle -or -not $Fixture) { continue }
        $env:WINDOWS_BUNDLE = $Bundle
        $env:WINDOWS_FIXTURE = $Fixture
        $arguments = @('-test.v','-test.timeout=7m','-test.run=^TestWindowsBundle$')
    }
    $stdout = Join-Path $TestDirectory ($test.BaseName + '.log')
    $stderr = Join-Path $TestDirectory ($test.BaseName + '.err')
    $process = Start-Process -FilePath $test.FullName -ArgumentList $arguments `
        -Wait -PassThru -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    Get-Content -Encoding UTF8 -LiteralPath $stdout
    Get-Content -Encoding UTF8 -LiteralPath $stderr
    if ($process.ExitCode -ne 0) { $failed = $true }
}
if ($failed) { throw 'Native Windows unit tests failed; see the per-package logs.' }
