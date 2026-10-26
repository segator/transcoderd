param(
 [Parameter(Mandatory=$true)][string]$Installer,
 [Parameter(Mandatory=$true)][string]$ServerURL,
 [Parameter(Mandatory=$true)][string]$TestBinary
)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'This destructive installer integration test is restricted to an isolated GitHub runner.' }
$install = Join-Path $env:RUNNER_TEMP 'TranscoderD setup test'
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$pipeName = 'transcoderd-desktop-' + $sid
function Invoke-Desktop([object]$request) {
 $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $pipeName, [IO.Pipes.PipeDirection]::InOut)
 try {
  $pipe.Connect(5000)
  $writer = [IO.StreamWriter]::new($pipe,[Text.UTF8Encoding]::new($false),1024,$true); $writer.AutoFlush=$true
  $reader = [IO.StreamReader]::new($pipe,[Text.Encoding]::UTF8,$false,1024,$true)
  $writer.WriteLine(($request | ConvertTo-Json -Compress -Depth 10))
  $reply = $reader.ReadLine() | ConvertFrom-Json
  if ($reply.error) { throw $reply.error }; return $reply.status
 } finally { $pipe.Dispose() }
}
function Wait-Worker([int]$limit) {
 for ($i=0; $i -lt 90; $i++) { $status=Invoke-Desktop @{command='status'}; if ($status.worker -and $status.worker.maxActiveJobs -eq $limit) { return $status }; Start-Sleep -Seconds 1 }
 throw 'Worker did not reach the desired limit.'
}
try {
 $p=Start-Process -FilePath $Installer -ArgumentList @('/S',('/D=' + $install)) -PassThru -Wait
 if ($p.ExitCode -ne 0) { throw "Installer failed: $($p.ExitCode)" }
 $service=Get-Service TranscoderDWorker; if ($service.Status -ne 'Running') { $service.WaitForStatus('Running',[TimeSpan]::FromSeconds(30)) }
 $status=Invoke-Desktop @{command='status'}; if ($status.configured) { throw 'Expected a clean service.' }
 $ui=Start-Process -FilePath (Join-Path $install 'transcoderd-tray.exe') -ArgumentList '--smoke' -PassThru
 $null=$ui.Handle
 if (-not $ui.WaitForExit(30000)) { Stop-Process -Id $ui.Id -Force; throw 'Tray web UI smoke test timed out.' }
 $ui.Refresh(); if ($ui.ExitCode -ne 0) { throw "Tray web UI smoke test failed: $($ui.ExitCode)" }
 $status=Invoke-Desktop @{command='configure';settings=@{domain=$ServerURL;token='e2e-test-token';name='desktop-e2e';codec='libsvtav1';maxJobs=0;schedule=@()}}
 if (-not $status.settings.tokenSet -or (($status | ConvertTo-Json -Depth 10) -match 'e2e-test-token')) { throw 'Token redaction failed.' }
 $null=Wait-Worker 0
 $null=Invoke-Desktop @{command='set-max-jobs';maxJobs=1}
 $null=Wait-Worker 1
 $env:E2E_SERVER_URL=$ServerURL; $env:E2E_TOKEN='e2e-test-token'; $env:E2E_SOURCE_PATH='e2e_fixture.mkv'
 $output = Join-Path $env:RUNNER_TEMP 'transcoderd-desktop-e2e.log'
 $errors = Join-Path $env:RUNNER_TEMP 'transcoderd-desktop-e2e.err'
 $test=Start-Process -FilePath $TestBinary -ArgumentList @('-test.v','-test.timeout=7m','-test.run=^TestE2E$') -PassThru -NoNewWindow -RedirectStandardOutput $output -RedirectStandardError $errors
 $null=$test.Handle; $reconfigured=$false; $sawJob=$false; $deadline=[DateTime]::UtcNow.AddMinutes(7)
 while (-not $test.HasExited -and [DateTime]::UtcNow -lt $deadline) {
  $status=Invoke-Desktop @{command='status'}
  if ($status.worker.jobs.Count -gt 0) { $sawJob=$true }
  if (-not $reconfigured -and $status.worker.activeJobs -gt 0) {
   $status=Invoke-Desktop @{command='configure';settings=@{domain=$ServerURL;token='';name='desktop-e2e';codec='libsvtav1';maxJobs=1;schedule=@('00:00-24:00')}}
   if (-not $status.pending) { throw 'Active job did not defer reconfiguration.' }; $reconfigured=$true
  }
  Start-Sleep -Milliseconds 250; $test.Refresh()
 }
 if (-not $test.HasExited) { Stop-Process -Id $test.Id -Force; throw 'Desktop E2E timed out.' }
 $test.WaitForExit(); $test.Refresh(); Get-Content $output; Get-Content $errors
 if ($test.ExitCode -ne 0 -or -not $reconfigured -or -not $sawJob) { throw 'Installed desktop service E2E failed.' }
 $applied=$false
 for ($i=0; $i -lt 90; $i++) { $status=Invoke-Desktop @{command='status'}; if (-not $status.pending -and $status.worker -and $status.settings.schedule -contains '00:00-24:00') { $applied=$true; break }; Start-Sleep -Seconds 1 }
 if (-not $applied) { throw 'Deferred settings did not apply after the encode.' }
 Write-Output 'Installed Windows service completed the SVT-AV1 CPU E2E and deferred reconfiguration.'
} finally {
 if (Test-Path (Join-Path $install 'uninstall.exe')) {
  $p=Start-Process -FilePath (Join-Path $install 'uninstall.exe') -ArgumentList '/S' -PassThru -Wait
  if ($p.ExitCode -ne 0) { Write-Warning "Uninstaller exited with $($p.ExitCode)." }
 }
}
