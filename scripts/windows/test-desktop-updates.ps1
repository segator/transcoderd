param(
 [Parameter(Mandatory=$true)][string]$FixtureDirectory,
 [Parameter(Mandatory=$true)][string]$ServerURL,
 [Parameter(Mandatory=$true)][string]$TestBinary,
 [Parameter(Mandatory=$true)][string]$SourceDirectory,
 [Parameter(Mandatory=$true)][string]$Fixture,
 [Parameter(Mandatory=$true)][string]$EvidenceDirectory
)
$ErrorActionPreference='Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'Installer update tests require an isolated GitHub runner.' }
$install=Join-Path $env:ProgramFiles 'TranscoderD'
$data=Join-Path $env:ProgramData 'TranscoderD'
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$feed=$null; $encode=$null; $tray=$null
function Invoke-DesktopRaw([object]$request) {
 $pipe=[IO.Pipes.NamedPipeClientStream]::new('.',('transcoderd-desktop-'+$sid),[IO.Pipes.PipeDirection]::InOut)
 try {
  $pipe.Connect(3000)
  $writer=[IO.StreamWriter]::new($pipe,[Text.UTF8Encoding]::new($false),1024,$true);$writer.AutoFlush=$true
  $reader=[IO.StreamReader]::new($pipe,[Text.Encoding]::UTF8,$false,1024,$true)
  $writer.WriteLine(($request | ConvertTo-Json -Compress -Depth 30))
  return ($reader.ReadLine() | ConvertFrom-Json)
 } finally { $pipe.Dispose() }
}
function Invoke-Desktop([object]$request) {
 $reply=Invoke-DesktopRaw $request
 if ($reply.error) { throw $reply.error };return $reply.status
}
function Wait-Status([scriptblock]$Match,[int]$Seconds=180) {
 $deadline=[DateTime]::UtcNow.AddSeconds($Seconds)
 while ([DateTime]::UtcNow -lt $deadline) {
  try { $current=Invoke-Desktop @{command='status'};if (& $Match $current) { return $current } } catch {}
  Start-Sleep -Milliseconds 250
 }
 throw 'Windows update state did not arrive before the deadline.'
}
function Set-Feed([bool]$Enabled,[bool]$BadSignature=$false,[bool]$BadDownload=$false) {
 $path=Join-Path $FixtureDirectory 'feed.json'
 @{enabled=$Enabled;badSignature=$BadSignature;badDownload=$BadDownload;version='v99.1.0'} | ConvertTo-Json | Set-Content -LiteralPath ($path+'.tmp') -Encoding utf8
 Move-Item -LiteralPath ($path+'.tmp') -Destination $path -Force
}
function Assert-Locked {
 foreach ($request in @(@{command='set-max-jobs';cpuJobs=9;gpuJobs=0},@{command='configure';settings=@{domain='http://wrong.invalid';token='changed';name='changed';cpuJobs=9;gpuJobs=0}})) {
  $reply=Invoke-DesktopRaw $request
  if ($reply.error -notmatch 'mandatory update' -or -not $reply.status.update.pending) { throw 'Direct IPC bypassed the mandatory configuration lock.' }
 }
}
function Assert-Owner {
 $registered=(Get-CimInstance Win32_Service -Filter "Name='TranscoderDWorker'").PathName
 if (-not $registered.Contains($sid)) { throw 'SYSTEM upgrade changed the original service owner.' }
 $saved=Get-Content -LiteralPath (Join-Path $data 'installation.json') -Raw | ConvertFrom-Json
 if ($saved.owner -ne $sid) { throw 'Installation metadata lost the original owner.' }
}
try {
 if (Get-Service TranscoderDWorker -ErrorAction SilentlyContinue) { throw 'Previous isolated installation was not removed.' }
 if (Test-Path $data) { Remove-Item -LiteralPath $data -Recurse -Force }
 $previousKey=$env:WINDOWS_UPDATE_SIGNING_KEY
 try {
  $env:WINDOWS_UPDATE_SIGNING_KEY=Get-Content -LiteralPath (Join-Path $FixtureDirectory 'key') -Raw
  & (Join-Path $FixtureDirectory 'sign.exe') --version v99.1.0 --installer (Join-Path $FixtureDirectory 'v99.1.0\transcoderd-worker-v99.1.0-windows-amd64-setup.exe') --files (Join-Path $FixtureDirectory 'v99.1.0\files.json') --output (Join-Path $FixtureDirectory 'v99.1.0')
  if ($LASTEXITCODE -ne 0) { throw 'Fixture signing failed.' }
 } finally { $env:WINDOWS_UPDATE_SIGNING_KEY=$previousKey }
 Set-Feed $false
 $feed=Start-Process -FilePath (Join-Path $FixtureDirectory 'feed.exe') -ArgumentList @('--root',('"'+$FixtureDirectory+'"')) -PassThru -NoNewWindow -RedirectStandardOutput (Join-Path $EvidenceDirectory 'feed.log') -RedirectStandardError (Join-Path $EvidenceDirectory 'feed.err')
 $p=Start-Process -FilePath (Join-Path $FixtureDirectory 'v99.0.0\transcoderd-worker-v99.0.0-windows-amd64-setup.exe') -ArgumentList '/S' -PassThru -Wait
 if ($p.ExitCode -ne 0) { throw "Bootstrap fixture installer failed: $($p.ExitCode)" }
 $null=Wait-Status {param($s) $s.version -eq 'v99.0.0'}
 Set-Feed $true $true
 $null=Invoke-Desktop @{command='check-update'}
 $status=Wait-Status {param($s) $s.update.phase -eq 'retrying' -and -not $s.update.pending}
 Write-Output 'Invalid signature rejected without establishing a pending update.'
 Set-Feed $false
 $null=Invoke-Desktop @{command='check-update'}
 $null=Wait-Status {param($s) $s.update.phase -eq 'current'}
 $settings=@{domain=$ServerURL;token='e2e-test-token';name='update-e2e';workingDirectory=(Join-Path $EvidenceDirectory 'encoding work');cpuCodec='libsvtav1';cpuJobs=1;gpuCodec='av1_nvenc';gpuJobs=0;schedule=@()}
 $null=Invoke-Desktop @{command='configure';settings=$settings}
 $null=Wait-Status {param($s) $s.worker -and $s.worker.maxActiveJobs -eq 1}
 $env:E2E_SERVER_URL=$ServerURL;$env:E2E_TOKEN='e2e-test-token';$env:E2E_SOURCE_PATH='e2e_fixture.mkv'
 $encode=Start-Process -FilePath $TestBinary -ArgumentList @('-test.v','-test.timeout=7m','-test.run=^TestE2E$') -PassThru -NoNewWindow -RedirectStandardOutput (Join-Path $EvidenceDirectory 'encode.log') -RedirectStandardError (Join-Path $EvidenceDirectory 'encode.err')
 $null=$encode.Handle
 $null=Wait-Status {param($s) $s.worker.activeJobs -gt 0}
 New-Item -ItemType File -Path (Join-Path $FixtureDirectory 'hold-download') -Force | Out-Null
 Set-Feed $true
 $null=Invoke-Desktop @{command='check-update'}
 $status=Wait-Status {param($s) $s.update.pending -and $s.update.phase -eq 'downloading' -and -not $s.worker.accepting -and $s.worker.reservedJobs -eq 0}
 Assert-Locked
 $ui=Start-Process -FilePath (Join-Path $install 'transcoderd-tray.exe') -ArgumentList '--smoke-update' -PassThru
 $null=$ui.Handle;if (-not $ui.WaitForExit(30000)) { Stop-Process -Id $ui.Id -Force;throw 'Locked update dashboard smoke test timed out.' };$ui.Refresh()
 if ($ui.ExitCode -ne 0) { throw 'Native WebView update banner/configuration lock failed.' }
 $tray=Start-Process -FilePath (Join-Path $install 'transcoderd-tray.exe') -ArgumentList '--background' -PassThru
 $null=$tray.Handle
 Copy-Item -LiteralPath $Fixture -Destination (Join-Path $SourceDirectory 'must-wait-for-update.mkv')
 $headers=@{Authorization='Bearer e2e-test-token'}
 $queued=Invoke-RestMethod -Method Post -Uri "$ServerURL/api/v1/job/" -Headers $headers -ContentType 'application/json' -Body '{"sourcePath":"must-wait-for-update.mkv"}'
 $queuedId=$queued.scheduled[0].id
 if (-not $queuedId) { throw 'Could not schedule the admission barrier fixture.' }
 if (-not $encode.WaitForExit(420000)) { throw 'Accepted encode did not finish during update drain.' };$encode.Refresh()
 Get-Content -LiteralPath (Join-Path $EvidenceDirectory 'encode.log');Get-Content -LiteralPath (Join-Path $EvidenceDirectory 'encode.err')
 if ($encode.ExitCode -ne 0) { throw 'Accepted SVT-AV1 pipeline did not complete its upload.' }
 $queuedJob=Invoke-RestMethod -Uri "$ServerURL/api/v1/job/$queuedId" -Headers $headers
 if ($queuedJob.status -ne 'queued') { throw 'Worker accepted a new assignment while the update installer was downloading.' }
 New-Item -ItemType File -Path (Join-Path $data 'updates\fixture-fail-install') -Force | Out-Null
 Remove-Item -LiteralPath (Join-Path $FixtureDirectory 'hold-download') -Force
 $status=Wait-Status {param($s) $s.version -eq 'v99.0.0' -and $s.update.pending -and $s.update.attempts -gt 0 -and $s.update.phase -eq 'retrying'} 300
 Set-Feed $true $true
 if (-not $tray.WaitForExit(10000)) { throw 'Updater did not close the original installed tray.' }
 Assert-Owner;Assert-Locked
 Write-Output 'Partial installer failure rolled back all installation files and retained the mandatory lock.'
 $journalPath=Join-Path $data 'updates\transaction.json'
 Stop-Service TranscoderDWorker;(Get-Service TranscoderDWorker).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
 $journal=Get-Content -LiteralPath $journalPath -Raw | ConvertFrom-Json
 $journal.phase='verifying';$journal.pid=0
 $journal | ConvertTo-Json -Depth 50 | Set-Content -LiteralPath $journalPath -Encoding utf8
 & schtasks.exe /Run /TN TranscoderDUpdateRecovery
 if ($LASTEXITCODE -ne 0) { throw 'Recovery task could not be invoked.' }
 $status=Wait-Status {param($s) $s.version -eq 'v99.0.0' -and $s.update.pending -and $s.update.phase -eq 'retrying'} 180
 Assert-Owner;Assert-Locked
 Remove-Item -LiteralPath (Join-Path $data 'updates\fixture-fail-install') -Force
 New-Item -ItemType File -Path (Join-Path $data 'updates\fixture-fail-health') -Force | Out-Null
 $attempts=$status.update.attempts
 Set-Feed $true
 $status=Wait-Status {param($s) $s.version -eq 'v99.0.0' -and $s.update.pending -and $s.update.phase -eq 'retrying' -and $s.update.attempts -gt $attempts} 300
 Set-Feed $true $true
 Assert-Locked;Assert-Owner
 Write-Output 'Unhealthy new service was rolled back without reopening admission.'
 Remove-Item -LiteralPath (Join-Path $data 'updates\fixture-fail-health') -Force
 New-Item -ItemType File -Path (Join-Path $data 'updates\fixture-restart-required') -Force | Out-Null
 Set-Feed $true
 $status=Wait-Status {param($s) $s.version -eq 'v99.1.0' -and $s.update.pending -and $s.update.phase -eq 'restart-required'} 300
 Assert-Locked;Assert-Owner
 # Exercise the post-boot branch through its real task entry point. A physical
 # reboot/no-logon drill is documented separately for an isolated VM.
 Stop-Service TranscoderDWorker;(Get-Service TranscoderDWorker).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
 $journal=Get-Content -LiteralPath $journalPath -Raw | ConvertFrom-Json
 $journal.bootTime=[DateTime]::UtcNow.AddYears(-1).ToString('o');$journal.pid=0
 $journal | ConvertTo-Json -Depth 50 | Set-Content -LiteralPath $journalPath -Encoding utf8
 & schtasks.exe /Run /TN TranscoderDUpdateRecovery
 if ($LASTEXITCODE -ne 0) { throw 'Post-restart recovery task could not run.' }
 $status=Wait-Status {param($s) $s.version -eq 'v99.1.0' -and -not $s.update.pending} 180
 Write-Output 'Restart-required state retained the lock; post-boot verification completed through the recovery task.'
 Assert-Owner
 foreach ($key in @('domain','name','workingDirectory','cpuCodec','cpuJobs','gpuCodec','gpuJobs')) { if ($status.settings.$key -ne $settings[$key]) { throw "Upgrade changed the saved setting: $key" } }
 if (-not $status.settings.tokenSet) { throw 'Upgrade lost the saved token.' }
 $null=Invoke-Desktop @{command='set-max-jobs';cpuJobs=0;gpuJobs=0}
 $null=Wait-Status {param($s) $s.worker -and $s.worker.activeJobs -eq 0 -and $s.worker.reservedJobs -eq 0} 180
 Write-Output 'Mandatory Windows update: signed release, immediate admission barrier, completed upload, locked WebView/IPC, rollback, recovery task and automatic retry passed.'
} finally {
 & schtasks.exe /End /TN TranscoderDUpdateRecovery 2>$null | Out-Null
 & schtasks.exe /Delete /TN TranscoderDUpdateRecovery /F 2>$null | Out-Null
 Get-CimInstance Win32_Process -Filter "Name='transcoderd-updater.exe'" | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith((Join-Path $data 'updates'),[StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
 if (Get-Service TranscoderDWorker -ErrorAction SilentlyContinue) { Stop-Service TranscoderDWorker -Force -ErrorAction SilentlyContinue }
 if ($encode -and -not $encode.HasExited) { Stop-Process -Id $encode.Id -Force }
 if (Test-Path (Join-Path $FixtureDirectory 'hold-download')) { Remove-Item -LiteralPath (Join-Path $FixtureDirectory 'hold-download') -Force }
 if (Test-Path (Join-Path $data 'updates')) {
  Get-ChildItem (Join-Path $data 'updates') -Recurse -File | Where-Object { $_.Extension -in @('.log','.err') } | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $EvidenceDirectory ($_.Directory.Name+'-'+$_.Name)) }
 }
 if ($tray -and -not $tray.HasExited) { Stop-Process -Id $tray.Id -Force }
 if ($feed -and -not $feed.HasExited) { Stop-Process -Id $feed.Id -Force }
 if (Test-Path (Join-Path $install 'uninstall.exe')) { $p=Start-Process -FilePath (Join-Path $install 'uninstall.exe') -ArgumentList '/S' -PassThru -Wait }
}
