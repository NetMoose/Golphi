# Windows PowerShell 5.1 compatible. Run in an interactive Windows desktop.
[CmdletBinding()]
param([string]$ArtifactDirectory = $PSScriptRoot)
$ErrorActionPreference = 'Stop'
$failed = $false
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$ArtifactDirectory = (Resolve-Path -LiteralPath $ArtifactDirectory).Path
$log = Join-Path $ArtifactDirectory "golphi-windows-mvp-$stamp.txt"
function Write-Log([string]$Message) {
    $Message | Out-File -LiteralPath $log -Encoding utf8 -Append
    Write-Host $Message
}
$names = @('GOLPHI_NATIVE_CORE', 'GOLPHI_NATIVE_WM_CLOSE', 'GOLPHI_RUN_TEST')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    Write-Log "Golphi Windows native verification; started $(Get-Date -Format o)"
    Write-Log "PowerShell: $($PSVersionTable.PSVersion); process 64-bit: $([Environment]::Is64BitProcess); OS 64-bit: $([Environment]::Is64BitOperatingSystem)"
    try {
        $os = Get-CimInstance Win32_OperatingSystem
        $cpus = Get-CimInstance Win32_Processor
        Write-Log "OS: $($os.Caption); version: $($os.Version); build: $($os.BuildNumber); architecture: $($os.OSArchitecture)"
        foreach ($cpu in $cpus) { Write-Log "CPU: $($cpu.Name); architecture code: $($cpu.Architecture) (9=AMD64, 12=ARM64); address width: $($cpu.AddressWidth)" }
        Write-Log "Environment architecture: $env:PROCESSOR_ARCHITECTURE; native architecture: $env:PROCESSOR_ARCHITEW6432"
    } catch { Write-Log "System information FAILED: $_"; $failed = $true }
    foreach ($name in @('hello-windows-amd64.exe', 'golphi-ui.test.exe', 'golphi-win32.test.exe')) {
        $path = Join-Path $ArtifactDirectory $name
        if (!(Test-Path -LiteralPath $path -PathType Leaf)) {
            Write-Log "MISSING artifact: $name"; $failed = $true
        } else {
            try { Write-Log "SHA256 $name $((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash)" }
            catch { Write-Log "SHA256 FAILED for ${name}: $_"; $failed = $true }
        }
    }
    $env:GOLPHI_NATIVE_CORE = '1'
    [Environment]::SetEnvironmentVariable('GOLPHI_NATIVE_WM_CLOSE', $null, 'Process')
    [Environment]::SetEnvironmentVariable('GOLPHI_RUN_TEST', $null, 'Process')
    Write-Log 'Test environment: GOLPHI_NATIVE_CORE=1; GOLPHI_NATIVE_WM_CLOSE and GOLPHI_RUN_TEST cleared.'
    foreach ($name in @('golphi-ui.test.exe', 'golphi-win32.test.exe')) {
        $path = Join-Path $ArtifactDirectory $name
        if (!(Test-Path -LiteralPath $path -PathType Leaf)) { Write-Log "SKIPPED missing suite: $name"; continue }
        $stdout = Join-Path $ArtifactDirectory "$name-$stamp.stdout.txt"
        $stderr = Join-Path $ArtifactDirectory "$name-$stamp.stderr.txt"
        Write-Log "RUN $name -test.v -test.timeout=90s"
        $process = $null
        try {
            $process = Start-Process -FilePath $path -ArgumentList @('-test.v', '-test.timeout=90s') -WorkingDirectory $ArtifactDirectory -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
            # Retain the native handle before exit (PowerShell 5.1 Process.ExitCode).
            $nativeHandle = $process.Handle
            if (!$process.WaitForExit(120000)) {
                Write-Log "WATCHDOG: $name exceeded 120 seconds; terminating test process."
                $process.Kill()
                $process.WaitForExit()
                $failed = $true
            } else { $process.WaitForExit() }
            $process.Refresh()
            $code = $process.ExitCode
            Write-Log "EXIT ${name}: $code"
            if ($null -eq $code -or $code -ne 0) { $failed = $true }
        } catch { Write-Log "EXECUTION FAILED for ${name}: $_"; $failed = $true }
        finally {
            foreach ($stream in @($stdout, $stderr)) {
                Write-Log "--- $([IO.Path]::GetFileName($stream)) ---"
                if (Test-Path -LiteralPath $stream) {
                    Get-Content -LiteralPath $stream -Encoding utf8 | ForEach-Object { Write-Log $_ }
                }
            }
            if ($null -ne $process) { $process.Dispose() }
        }
    }
} catch { Write-Log "Verification FAILED: $_"; $failed = $true }
finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
    Write-Log "Finished $(Get-Date -Format o); failed=$failed; log=$log"
    Write-Log 'Send the complete log even on failure. Manual hello checks are still required.'
}
if ($failed) { exit 1 }
exit 0
