param(
    [Parameter(Mandatory=$true)][string]$Bundle,
    [Parameter(Mandatory=$true)][string]$TestBinary,
    [Parameter(Mandatory=$true)][string]$ServerBinary,
    [Parameter(Mandatory=$true)][string]$Fixture,
    [Parameter(Mandatory=$true)][string]$EvidenceDirectory,
    [string]$PostgresBin = $env:PGBIN,
    [string]$DesktopInstaller,
    [string]$UpdateFixtures
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$env:PATH = "$env:SystemRoot\System32;$env:SystemRoot;" + $env:PATH
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
if (-not $PostgresBin) { throw 'Set PGBIN or -PostgresBin to the PostgreSQL bin directory.' }
foreach ($tool in @('initdb','pg_ctl','createdb')) {
    if (-not (Test-Path (Join-Path $PostgresBin ($tool+'.exe')))) { throw "PostgreSQL tool missing: $tool" }
}
New-Item -ItemType Directory -Path $EvidenceDirectory -Force | Out-Null
$EvidenceDirectory = (Resolve-Path -LiteralPath $EvidenceDirectory).Path
$run = Join-Path $env:TEMP ('TranscoderD CI ' + [guid]::NewGuid().ToString('N'))
$serverHome = Join-Path $run 'server'
$source = Join-Path $run 'server media'
$data = Join-Path $run 'postgres data'
New-Item -ItemType Directory -Path $serverHome,$source,(Join-Path $serverHome 'bin') | Out-Null
Copy-Item -LiteralPath $ServerBinary -Destination (Join-Path $serverHome 'server.exe')
Copy-Item -LiteralPath (Join-Path $Bundle 'bin\ffprobe.exe') -Destination (Join-Path $serverHome 'bin\ffprobe.exe')
Copy-Item -LiteralPath $Fixture -Destination (Join-Path $source 'e2e_fixture.mkv')

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback,0)
    $listener.Start()
    try { return $listener.LocalEndpoint.Port } finally { $listener.Stop() }
}
function Format-Arguments([string[]]$Values) {
    return (($Values | ForEach-Object { '"' + $_.Replace('"','\"') + '"' }) -join ' ')
}
function Invoke-Program([string]$File,[string[]]$Values,[string]$Name) {
    $out = Join-Path $EvidenceDirectory ($Name+'.log')
    $err = Join-Path $EvidenceDirectory ($Name+'.err')
    $p = Start-Process -FilePath $File -ArgumentList (Format-Arguments $Values) -WorkingDirectory $run `
        -PassThru -NoNewWindow -RedirectStandardOutput $out -RedirectStandardError $err
    # Keep the handle open so Windows PowerShell can read ExitCode after exit.
    $null = $p.Handle
    # -Wait also waits for pg_ctl's persistent PostgreSQL descendants.
    $p.WaitForExit()
    $p.Refresh()
    Get-Content -Encoding UTF8 -LiteralPath $out
    Get-Content -Encoding UTF8 -LiteralPath $err
    if ($p.ExitCode -ne 0) { throw "$Name exited with $($p.ExitCode)" }
}
$dbPort = Get-FreePort
$httpPort = Get-FreePort
while ($httpPort -eq $dbPort) { $httpPort = Get-FreePort }
$server = $null
$databaseStarted = $false
$serverLog = Join-Path $EvidenceDirectory 'server.log'
$serverErr = Join-Path $EvidenceDirectory 'server.err'
try {
    Invoke-Program (Join-Path $PostgresBin 'initdb.exe') @('-D',$data,'-U','test','-A','trust','--encoding=UTF8','--no-locale') 'initdb'
    # This cluster contains only the fixture and listens only on loopback.
    # It does not use or alter the runner's installed PostgreSQL service.
    $databaseStarted = $true
    Invoke-Program (Join-Path $PostgresBin 'pg_ctl.exe') @('-D',$data,'-l',(Join-Path $EvidenceDirectory 'postgres.log'),'-o',"-p $dbPort -h 127.0.0.1",'-w','start') 'postgres-start'
    Invoke-Program (Join-Path $PostgresBin 'createdb.exe') @('-h','127.0.0.1','-p',"$dbPort",'-U','test','transcoderd_e2e') 'createdb'
    $values = @('--noUpdates','--web.port',"$httpPort",'--web.token','e2e-test-token',
        '--server.database.Host','127.0.0.1','--server.database.port',"$dbPort",'--server.database.User','test',
        '--server.database.Password','test','--server.database.Scheme','transcoderd_e2e',
        '--server.scheduler.sourcePath',$source,'--server.scheduler.minFileSize','100',
        '--server.scheduler.scheduleTime','5s','--server.scheduler.jobTimeout','10m')
    $previousPath = $env:PATH
    try {
        # Server output verification also invokes the probe library via PATH.
        $env:PATH = (Join-Path $serverHome 'bin') + ';' + $previousPath
        $server = Start-Process -FilePath (Join-Path $serverHome 'server.exe') -ArgumentList (Format-Arguments $values) `
            -WorkingDirectory $run -PassThru -NoNewWindow -RedirectStandardOutput $serverLog -RedirectStandardError $serverErr
    }
    finally { $env:PATH = $previousPath }
    if ($UpdateFixtures) {
        & (Join-Path $PSScriptRoot 'test-desktop-updates.ps1') -FixtureDirectory $UpdateFixtures -ServerURL "http://127.0.0.1:$httpPort" -TestBinary $TestBinary -SourceDirectory $source -Fixture $Fixture -EvidenceDirectory $EvidenceDirectory
    } elseif ($DesktopInstaller) {
        & (Join-Path $PSScriptRoot 'test-desktop-installer.ps1') -Installer $DesktopInstaller -ServerURL "http://127.0.0.1:$httpPort" -TestBinary $TestBinary
    } else {
        & (Join-Path $PSScriptRoot 'test-worker.ps1') -Bundle $Bundle -TestBinary $TestBinary `
            -ServerURL "http://127.0.0.1:$httpPort" -Codec libsvtav1 -EvidenceDirectory (Join-Path $EvidenceDirectory 'worker')
    }
    Write-Output 'Hosted-runner Windows SVT-AV1 CPU E2E passed.'
}
finally {
    if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force; $server.WaitForExit() }
    if ($databaseStarted) {
        Invoke-Program (Join-Path $PostgresBin 'pg_ctl.exe') @('-D',$data,'-m','immediate','-w','stop') 'postgres-stop'
    }
    if ($DesktopInstaller -and (Test-Path "$env:ProgramData\TranscoderD\worker.log")) {
        Copy-Item "$env:ProgramData\TranscoderD\worker.log" (Join-Path $EvidenceDirectory 'desktop-worker.log')
    }
    if (Test-Path $serverLog) { Get-Content -Encoding UTF8 -LiteralPath $serverLog }
    if (Test-Path $serverErr) { Get-Content -Encoding UTF8 -LiteralPath $serverErr }
    Write-Output "CI E2E evidence: $EvidenceDirectory"
}
