param(
    [Parameter(Mandatory=$true)][string]$Bundle,
    [Parameter(Mandatory=$true)][string]$TestBinary,
    [Parameter(Mandatory=$true)][string]$ServerURL,
    [ValidateSet('libsvtav1','av1_nvenc')][string]$Codec = 'libsvtav1'
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$env:PATH = "$env:SystemRoot\System32;$env:SystemRoot;" + $env:PATH
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$run = Join-Path $env:TEMP ('TranscoderD prueba pel' + [char]0xed + 'cula ' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $run | Out-Null
$install = Join-Path $run 'bundle con espacios'
Copy-Item -LiteralPath $Bundle -Destination $install -Recurse
$worker = Join-Path $install 'transcoderd-worker.exe'
$runner = Join-Path $run 'e2e.test.exe'
Copy-Item -LiteralPath $TestBinary -Destination $runner
$env:E2E_SERVER_URL = $ServerURL
$env:E2E_TOKEN = 'e2e-test-token'
$env:E2E_SOURCE_PATH = 'e2e_fixture.mkv'
$workerLog = Join-Path $run 'worker.log'
$workerErr = Join-Path $run 'worker.err'

function Invoke-Checked([string]$File, [string[]]$Arguments, [string]$Name) {
    $out = Join-Path $run ($Name + '.log')
    $err = Join-Path $run ($Name + '.err')
    $p = Start-Process -FilePath $File -ArgumentList $Arguments -WorkingDirectory $run `
        -Wait -PassThru -NoNewWindow -RedirectStandardOutput $out -RedirectStandardError $err
    Get-Content -Encoding UTF8 -LiteralPath $out
    Get-Content -Encoding UTF8 -LiteralPath $err
    if ($p.ExitCode -ne 0) { throw "$Name exited with $($p.ExitCode). Logs: $run" }
}

$process = $null
try {
    $headers = @{ Authorization='Bearer e2e-test-token'; workerName='windows-readiness' }
    $ready = $false
    for ($attempt=0; $attempt -lt 60; $attempt++) {
        try { Invoke-WebRequest -UseBasicParsing -Uri "$ServerURL/api/v1/job/request" -Headers $headers -TimeoutSec 3 | Out-Null; $ready=$true; break }
        catch { Start-Sleep -Milliseconds 500 }
    }
    if (-not $ready) { throw 'Linux E2E server did not become ready' }
    $arguments = @('--noUpdates','--web.domain',$ServerURL,'--web.token','e2e-test-token',
        '--worker.name','windows-e2e','--worker.maxActiveJobs','1',
        '--worker.ffmpegConfig.videoCodec',$Codec,'--worker.ffmpegConfig.threads','2',
        '--worker.verifyDeltaTime','5','--worker.temporalPath',('"'+(Join-Path $run ('trabajo pel' + [char]0xed + 'cula'))+'"'))
    $process = Start-Process -FilePath $worker -ArgumentList $arguments -WorkingDirectory $run `
        -PassThru -NoNewWindow -RedirectStandardOutput $workerLog -RedirectStandardError $workerErr
    $controlReady = $false
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        $process.Refresh()
        if ($process.HasExited) { throw "Worker exited with $($process.ExitCode)" }
        try { Invoke-Checked $worker @('control','status') 'control-status'; $controlReady=$true; break }
        catch { Start-Sleep -Milliseconds 500 }
    }
    if (-not $controlReady) { throw 'Windows control pipe did not become ready' }
    Invoke-Checked $worker @('control','set-max-jobs','0') 'control-pause'
    Invoke-Checked $worker @('control','set-max-jobs','1') 'control-resume'
    Invoke-Checked $runner @('-test.v','-test.timeout=7m','-test.run=^TestE2E$') 'e2e'
    Write-Output "Native Windows $Codec E2E passed. Evidence: $run"
}
finally {
    if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
    if (Test-Path $workerLog) { Get-Content -Encoding UTF8 -LiteralPath $workerLog }
    if (Test-Path $workerErr) { Get-Content -Encoding UTF8 -LiteralPath $workerErr }
    Write-Output "Evidence retained at $run"
}
