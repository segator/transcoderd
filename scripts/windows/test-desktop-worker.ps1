param(
 [Parameter(Mandatory=$true)][string]$Bundle,
 [Parameter(Mandatory=$true)][string]$WorkerBinary,
 [Parameter(Mandatory=$true)][string]$ServiceBinary,
 [Parameter(Mandatory=$true)][string]$TestBinary,
 [Parameter(Mandatory=$true)][string]$ServerURL
)
$ErrorActionPreference = 'Stop'
$env:PATH = "$env:SystemRoot\System32;$env:SystemRoot;" + $env:PATH
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$run = Join-Path $env:TEMP ('TranscoderD desktop e2e ' + [guid]::NewGuid().ToString('N'))
$install = Join-Path $run 'bundle'
New-Item -Type Directory -Path $install,(Join-Path $run 'settings') | Out-Null
foreach ($directory in @('bin','pgs','tessdata')) {
 & cmd.exe /c "mklink /J `"$install\$directory`" `"$Bundle\$directory`"" | Out-Null
 if ($LASTEXITCODE -ne 0) { throw "Could not create isolated bundle junction for $directory" }
}
Copy-Item -LiteralPath $WorkerBinary -Destination (Join-Path $install 'transcoderd-worker.exe')
$suffix = '-' + [guid]::NewGuid().ToString('N')
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$env:TRANSCODERD_TEST_PIPE_SUFFIX = $suffix
$process=$null
function Request([object]$value) {
 $pipe=[IO.Pipes.NamedPipeClientStream]::new('.',('transcoderd-desktop-'+$sid+$suffix),[IO.Pipes.PipeDirection]::InOut)
 try { $pipe.Connect(3000); $writer=[IO.StreamWriter]::new($pipe,[Text.UTF8Encoding]::new($false),1024,$true); $writer.AutoFlush=$true; $reader=[IO.StreamReader]::new($pipe,[Text.Encoding]::UTF8,$false,1024,$true); $writer.WriteLine(($value | ConvertTo-Json -Compress -Depth 10)); $response=$reader.ReadLine() | ConvertFrom-Json; if ($response.error) { throw $response.error }; return $response.status } finally { $pipe.Dispose() }
}
try {
 $process=Start-Process -FilePath $ServiceBinary -ArgumentList @('--console',('"'+(Join-Path $run 'settings')+'"'),('"'+(Join-Path $install 'transcoderd-worker.exe')+'"')) -PassThru -WindowStyle Hidden
 $ready=$false
 for ($i=0; $i -lt 60; $i++) { try { $null=Request @{command='status'}; $ready=$true; break } catch { Start-Sleep -Milliseconds 500 } }
 if (-not $ready) { throw 'Desktop supervisor did not start.' }
 $status=Request @{command='configure';settings=@{domain=$ServerURL;token='e2e-test-token';name='desktop-local-e2e';codec='libsvtav1';maxJobs=0;schedule=@()}}
 $ready=$false
 for ($i=0; $i -lt 90; $i++) { $status=Request @{command='status'}; if ($status.worker -and $status.worker.maxActiveJobs -eq 0) { $ready=$true; break }; Start-Sleep -Seconds 1 }
 if (-not $ready) { throw "Paused worker not ready: $($status | ConvertTo-Json -Depth 10)" }
 $null=Request @{command='set-max-jobs';maxJobs=1}
 $env:E2E_SERVER_URL=$ServerURL; $env:E2E_TOKEN='e2e-test-token'; $env:E2E_SOURCE_PATH='e2e_fixture.mkv'
 $runner = Join-Path $run 'e2e.test.exe'; Copy-Item -LiteralPath $TestBinary -Destination $runner
 $out = Join-Path $run 'e2e.log'; $err = Join-Path $run 'e2e.err'
 $test = Start-Process -FilePath $runner -ArgumentList @('-test.v','-test.timeout=7m','-test.run=^TestE2E$') -WorkingDirectory $run -PassThru -NoNewWindow -RedirectStandardOutput $out -RedirectStandardError $err
 $null=$test.Handle
 $reconfigured=$false; $sawJob=$false; $deadline=[DateTime]::UtcNow.AddMinutes(7)
 while (-not $test.HasExited -and [DateTime]::UtcNow -lt $deadline) {
  $status=Request @{command='status'}
  if ($status.worker.jobs.Count -gt 0) { $sawJob=$true }
  if (-not $reconfigured -and $status.worker.activeJobs -gt 0) {
   $status=Request @{command='configure';settings=@{domain=$ServerURL;token='';name='desktop-local-e2e';codec='libsvtav1';maxJobs=1;schedule=@('00:00-24:00')}}
   if (-not $status.pending) { throw 'Configuration was not deferred during an active job.' }
   $reconfigured=$true
  }
  Start-Sleep -Milliseconds 250
  $test.Refresh()
 }
 if (-not $test.HasExited) { Stop-Process -Id $test.Id -Force; throw 'Desktop worker E2E timed out.' }
 $test.WaitForExit(); $test.Refresh(); Get-Content $out; Get-Content $err
 if ($test.ExitCode -ne 0) { throw 'Desktop worker CPU E2E failed.' }
 if (-not $reconfigured -or -not $sawJob) { throw 'Did not observe a job and deferred reconfiguration.' }
 $applied=$false
 for ($i=0; $i -lt 90; $i++) { $status=Request @{command='status'}; if (-not $status.pending -and $status.worker -and $status.settings.schedule -contains '00:00-24:00') { $applied=$true; break }; Start-Sleep -Seconds 1 }
 if (-not $applied) { throw 'Deferred settings were not applied after the job finished.' }
 Write-Output 'Windows desktop supervisor completed the isolated SVT-AV1 CPU E2E and deferred reconfiguration.'
} finally {
 if ($process -and -not $process.HasExited) { try { $null=Request @{command='shutdown-test'}; if (-not $process.WaitForExit(15000)) { throw 'Graceful shutdown timed out' } } catch { Stop-Process -Id $process.Id -Force; throw } }
 Remove-Item Env:TRANSCODERD_TEST_PIPE_SUFFIX -ErrorAction SilentlyContinue
 Write-Output "Desktop E2E logs: $run"
}
