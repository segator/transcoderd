param(
    [Parameter(Mandatory=$true)][string]$Executable,
    [Parameter(Mandatory=$true)][string]$ArgumentsFile,
    [Parameter(Mandatory=$true)][string]$ResultFile
)
$ErrorActionPreference = 'Stop'
$values = Get-Content -Raw -Encoding UTF8 -LiteralPath $ArgumentsFile | ConvertFrom-Json
$arguments = (($values | ForEach-Object { '"' + $_.Replace('"','\"') + '"' }) -join ' ')
$stdout = $ResultFile + '.stdout'
$stderr = $ResultFile + '.stderr'
$watch = [System.Diagnostics.Stopwatch]::StartNew()
$p = Start-Process -FilePath $Executable -ArgumentList $arguments -PassThru -NoNewWindow `
    -RedirectStandardOutput $stdout -RedirectStandardError $stderr
$null = $p.Handle
$p.WaitForExit()
$watch.Stop()
$p.Refresh()
$result = @{
    exit_code = $p.ExitCode
    wall_seconds = $watch.Elapsed.TotalSeconds
    cpu_seconds = $p.TotalProcessorTime.TotalSeconds
}
$result | ConvertTo-Json | Set-Content -Encoding UTF8 -LiteralPath $ResultFile
if ($p.ExitCode -ne 0) { Get-Content -Encoding UTF8 -LiteralPath $stderr; throw "Encoder exited with $($p.ExitCode)" }
