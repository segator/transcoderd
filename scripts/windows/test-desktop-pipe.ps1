param([Parameter(Mandatory=$true)][string]$ServiceBinary)
$ErrorActionPreference = 'Stop'
$run = Join-Path $env:TEMP ('TranscoderD desktop test ' + [guid]::NewGuid().ToString('N'))
New-Item -Type Directory -Path $run | Out-Null
$suffix = '-' + [guid]::NewGuid().ToString('N')
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$env:TRANSCODERD_TEST_PIPE_SUFFIX = $suffix
$process = $null
function Request([object]$value) {
  $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', ('transcoderd-desktop-' + $sid + $suffix), [IO.Pipes.PipeDirection]::InOut)
  try {
    $pipe.Connect(3000)
    $writer = [IO.StreamWriter]::new($pipe, [Text.UTF8Encoding]::new($false), 1024, $true)
    $writer.AutoFlush = $true
    $reader = [IO.StreamReader]::new($pipe, [Text.Encoding]::UTF8, $false, 1024, $true)
    $writer.WriteLine(($value | ConvertTo-Json -Compress -Depth 10))
    return ($reader.ReadLine() | ConvertFrom-Json)
  } finally { $pipe.Dispose() }
}
try {
  $process = Start-Process -FilePath $ServiceBinary -ArgumentList @('--console', ('"' + $run + '"'), ('"' + (Join-Path $run 'missing-worker.exe') + '"')) -PassThru -WindowStyle Hidden
  $ready=$false
  for ($i=0; $i -lt 20; $i++) { try { $result=Request @{command='status'}; $ready=$true; break } catch { Start-Sleep -Milliseconds 250 } }
  if (-not $ready -or $result.status.configured) { throw 'Service did not start unconfigured.' }
  $invalid = Request @{command='configure';settings=@{domain='http://127.0.0.1:1';token='secret';name='worker';cpuCodec='libsvtav1';cpuJobs=1;gpuCodec='invalid';gpuJobs=1;schedule=@()}}
  if (-not $invalid.error) { throw 'Invalid codec was accepted.' }
  $response = Request @{command='configure';settings=@{domain='http://127.0.0.1:1';token='private-test-token';name='worker';cpuCodec='libsvtav1';cpuJobs=0;gpuCodec='av1_nvenc';gpuJobs=0;schedule=@()}}
  if ($response.error -or -not $response.status.configured -or -not $response.status.settings.tokenSet) { throw "Could not save settings: $($response.error)" }
  $json = $response | ConvertTo-Json -Depth 10
  if ($json -match 'private-test-token') { throw 'Token leaked in response.' }
  $state = Get-Content -LiteralPath (Join-Path $run 'service.json') -Raw | ConvertFrom-Json
  if ($state.active.token -ne 'private-test-token' -or $state.active.cpuJobs -ne 0 -or $state.active.gpuJobs -ne 0) { throw 'Settings were not persisted.' }
  Write-Output 'Windows desktop pipe: validation, persistence and secret redaction passed.'
} finally {
  Remove-Item Env:TRANSCODERD_TEST_PIPE_SUFFIX -ErrorAction SilentlyContinue
  if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id -Force; $process.WaitForExit() }
  Remove-Item -LiteralPath $run -Recurse -Force -ErrorAction SilentlyContinue
}
