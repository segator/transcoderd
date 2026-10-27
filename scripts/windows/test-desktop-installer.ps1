param(
 [Parameter(Mandatory=$true)][string]$Installer,
 [Parameter(Mandatory=$true)][string]$ServerURL,
 [Parameter(Mandatory=$true)][string]$TestBinary
)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'This destructive installer integration test is restricted to an isolated GitHub runner.' }
$install = Join-Path $env:ProgramFiles 'TranscoderD'
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$pipeName = 'transcoderd-desktop-' + $sid
$firewallName = 'TranscoderD-Installer-E2E-' + [guid]::NewGuid().ToString('N')
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
 # The production installer must retain its real trust/endpoint policy. Isolate
 # this ordinary service test from public releases using runner networking;
 # mandatory upgrade behavior is exercised separately with signed fixtures.
 New-NetFirewallRule -Name $firewallName -DisplayName $firewallName -Direction Outbound `
  -Program (Join-Path $install 'transcoderd-desktop-service.exe') -Action Block -Profile Any `
  -RemoteAddress @('0.0.0.0-126.255.255.255','128.0.0.0-255.255.255.255','::/0') | Out-Null
 $p=Start-Process -FilePath $Installer -ArgumentList '/S' -PassThru -Wait
 if ($p.ExitCode -ne 0) { throw "Installer failed: $($p.ExitCode)" }
 $service=Get-Service TranscoderDWorker; if ($service.Status -ne 'Running') { $service.WaitForStatus('Running',[TimeSpan]::FromSeconds(30)) }
 $status=Invoke-Desktop @{command='status'}; if ($status.configured) { throw 'Expected a clean service.' }
 Add-Type -AssemblyName System.Drawing
 $trayExe = Join-Path $install 'transcoderd-tray.exe'
 $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($trayExe)
 if ($null -eq $icon -or $icon.Width -lt 16 -or $icon.Height -lt 16) { throw 'Tray executable does not contain a usable application icon.' }
 $icon.Dispose()
 $ui=Start-Process -FilePath $trayExe -ArgumentList '--smoke' -PassThru
 $diagnostic = Join-Path $env:TEMP ("TranscoderD-tray-smoke-{0}.log" -f $ui.Id)
 Remove-Item -LiteralPath $diagnostic -Force -ErrorAction SilentlyContinue
 $null=$ui.Handle
 if (-not $ui.WaitForExit(30000)) { Stop-Process -Id $ui.Id -Force; throw 'Tray web UI smoke test timed out.' }
 $ui.Refresh(); if ($ui.ExitCode -ne 0) {
  if (Test-Path $diagnostic) { Get-Content -LiteralPath $diagnostic }
  throw "Tray web UI smoke test failed: $($ui.ExitCode)"
 }
 Stop-Service TranscoderDWorker; (Get-Service TranscoderDWorker).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
 $fakeDir = Join-Path $env:RUNNER_TEMP ('TranscoderD fake service ' + [guid]::NewGuid().ToString('N'))
 New-Item -ItemType Directory -Path $fakeDir | Out-Null
 $fake = Start-Process -FilePath (Join-Path $install 'transcoderd-desktop-service.exe') -ArgumentList @('--console',('"'+$fakeDir+'"'),('"'+$fakeDir+'\missing-worker.exe"')) -PassThru
 try {
  $ready=$false
  for ($i=0; $i -lt 30; $i++) { try { $null=Invoke-Desktop @{command='status'}; $ready=$true; break } catch { Start-Sleep -Milliseconds 250 } }
  if (-not $ready) { throw 'Fake test service did not start.' }
  $ui=Start-Process -FilePath (Join-Path $install 'transcoderd-tray.exe') -ArgumentList '--smoke-reject' -PassThru
  $null=$ui.Handle
  if (-not $ui.WaitForExit(30000)) { Stop-Process -Id $ui.Id -Force; throw 'Pipe impersonation test timed out.' }
  $ui.Refresh(); if ($ui.ExitCode -ne 0) { throw 'Tray did not reject the fake service pipe.' }
 } finally { if (-not $fake.HasExited) { Stop-Process -Id $fake.Id -Force; $fake.WaitForExit() } }
 Start-Service TranscoderDWorker; (Get-Service TranscoderDWorker).WaitForStatus('Running',[TimeSpan]::FromSeconds(30))
 $status=Invoke-Desktop @{command='configure';settings=@{domain=$ServerURL;token='e2e-test-token';name='desktop-e2e';cpuCodec='libsvtav1';cpuJobs=0;gpuCodec='av1_nvenc';gpuJobs=0;schedule=@()}}
 if (-not $status.settings.tokenSet -or (($status | ConvertTo-Json -Depth 10) -match 'e2e-test-token')) { throw 'Token redaction failed.' }
 $settingsPath = Join-Path $env:ProgramData 'TranscoderD\service.json'
 $acl = Get-Acl -LiteralPath $settingsPath
 foreach ($entry in $acl.Access) {
  if ($entry.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Allow) { continue }
  $account = $entry.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value
  if ($account -notin @('S-1-5-18','S-1-5-32-544')) { throw "Settings grant access to an unexpected account: $account" }
 }
 $null=Wait-Worker 0
 $null=Invoke-Desktop @{command='set-max-jobs';cpuJobs=1;gpuJobs=0}
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
   $status=Invoke-Desktop @{command='configure';settings=@{domain=$ServerURL;token='';name='desktop-e2e';cpuCodec='libsvtav1';cpuJobs=1;gpuCodec='av1_nvenc';gpuJobs=0;schedule=@('00:00-24:00')}}
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
 Remove-NetFirewallRule -Name $firewallName -ErrorAction SilentlyContinue
 if (Test-Path (Join-Path $install 'uninstall.exe')) {
  $p=Start-Process -FilePath (Join-Path $install 'uninstall.exe') -ArgumentList '/S' -PassThru -Wait
  if ($p.ExitCode -ne 0) { Write-Warning "Uninstaller exited with $($p.ExitCode)." }
 }
}
