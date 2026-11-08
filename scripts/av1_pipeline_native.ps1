param([Parameter(Mandatory=$true)][string]$ConfigPath)
$ErrorActionPreference = 'Stop'
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
Set-Location -LiteralPath $config.directory
function Launch-Encode($arguments, $name) {
    $info = New-Object System.Diagnostics.ProcessStartInfo
    $info.FileName = $config.ffmpeg
    $info.Arguments = $arguments
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $info
    [void]$process.Start()
    $stdout = $process.StandardOutput.ReadToEndAsync()
    $stderr = $process.StandardError.ReadToEndAsync()
    return [PSCustomObject]@{ Process=$process; Stdout=$stdout; Stderr=$stderr; Name=$name }
}
function Save-Logs($handle) {
    [IO.File]::WriteAllText((Join-Path $config.directory ($handle.Name + '.stdout.log')), $handle.Stdout.GetAwaiter().GetResult())
    [IO.File]::WriteAllText((Join-Path $config.directory ($handle.Name + '.stderr.log')), $handle.Stderr.GetAwaiter().GetResult())
}
function Frame-Count($path) {
    if (!(Test-Path -LiteralPath $path)) { return 0 }
    $frameMatches = [regex]::Matches([IO.File]::ReadAllText($path), '(?m)^frame=(\d+)\r?$')
    if ($frameMatches.Count -eq 0) { return 0 }
    return [int]$frameMatches[$frameMatches.Count - 1].Groups[1].Value
}
$results = @()
if (Test-Path -LiteralPath 'native-results.json') {
    $results = @(Get-Content -LiteralPath 'native-results.json' -Raw | ConvertFrom-Json)
}
foreach ($round in $config.rounds) {
    $previous = @($results | Where-Object { $_.name -eq $round.name })
    if ($previous.Count -gt 0) {
        foreach ($job in $round.jobs) {
            $outputPath = [IO.Path]::ChangeExtension($job.progress, '.mkv')
            if (!(Test-Path -LiteralPath $outputPath) -or (Frame-Count $job.progress) -ne $config.frames) {
                throw ('Missing or incomplete cached native output: ' + $job.name)
            }
        }
        Write-Output ('REUSE NATIVE ' + $round.name)
        continue
    }
    Write-Output ('NATIVE ' + $round.name)
    $loads = @()
    $gpus = @()
    try {
        $watch = [Diagnostics.Stopwatch]::StartNew()
        $effectiveCpuArguments = @()
        foreach ($job in $round.loads) {
            $cpuArguments = $job.arguments -replace ' -f null -$', (' -frames:v ' + $config.frames + ' -f null -')
            if ($cpuArguments -eq $job.arguments) { throw 'CPU arguments must end with -f null -' }
            $effectiveCpuArguments += $cpuArguments
            $loads += Launch-Encode $cpuArguments $job.name
        }
        foreach ($job in $round.jobs) { $gpus += Launch-Encode $job.arguments $job.name }
        $all = @($loads) + @($gpus)
        $ends = @{}
        while ($ends.Count -lt $all.Count) {
            for ($index=0; $index -lt $all.Count; $index++) {
                $process = $all[$index].Process
                if (!$ends.ContainsKey($index) -and $process.HasExited) {
                    $process.WaitForExit()
                    if ($process.ExitCode -ne 0) { throw ('Encode failed: ' + $all[$index].Name) }
                    $ends[$index] = $watch.Elapsed.TotalSeconds
                }
            }
            Start-Sleep -Milliseconds 20
        }
        $seconds = ($ends.Values | Measure-Object -Maximum).Maximum
        $cpuEnds = @()
        $gpuEnds = @()
        for ($index=0; $index -lt $loads.Count; $index++) { $cpuEnds += $ends[$index] }
        for ($index=0; $index -lt $gpus.Count; $index++) { $gpuEnds += $ends[$index + $loads.Count] }
        foreach ($job in @($round.loads) + @($round.jobs)) {
            if ((Frame-Count $job.progress) -ne $config.frames) { throw ('Frame count mismatch: ' + $job.name) }
        }
        $cpuComputeSeconds = 0.0
        foreach ($handle in $loads) { $cpuComputeSeconds += $handle.Process.TotalProcessorTime.TotalSeconds }
        $gpuCpuSeconds = 0.0
        foreach ($handle in $gpus) { $gpuCpuSeconds += $handle.Process.TotalProcessorTime.TotalSeconds }
        $row = [PSCustomObject]@{ name=$round.name; mode=$round.mode; repeat=$round.repeat;
            cpu_jobs=$loads.Count; gpu_jobs=$gpus.Count; seconds=$seconds; batch_seconds=$seconds;
            frames_per_job=$config.frames; fixed_total_frames=($config.frames*$all.Count);
            cpu_encode_seconds=(($cpuEnds | Measure-Object -Maximum).Maximum);
            gpu_encode_seconds=(($gpuEnds | Measure-Object -Maximum).Maximum);
            gpu_fps=($config.frames*$gpus.Count/$seconds); cpu_fps=($config.frames*$loads.Count/$seconds);
            cpu_seconds=$cpuComputeSeconds; cpu_percent=($cpuComputeSeconds/$seconds*100);
            gpu_cpu_seconds=$gpuCpuSeconds; total_cpu_seconds=($cpuComputeSeconds+$gpuCpuSeconds);
            effective_cpu_arguments=$effectiveCpuArguments }
        $results += $row
        $results | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath 'native-results.json' -Encoding UTF8
        Write-Output ($row | ConvertTo-Json -Compress)
    }
    finally {
        foreach ($handle in @($loads) + @($gpus)) {
            if (!$handle.Process.HasExited) { $handle.Process.Kill() }
            $handle.Process.WaitForExit()
            Save-Logs $handle
            $handle.Process.Dispose()
        }
    }
}
Write-Output 'NATIVE COMPLETE'
