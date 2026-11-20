param(
    [string]$Probe = "$PSScriptRoot\probe-tree-windows.exe",
    [string]$LogDirectory = "$PSScriptRoot\dynamic-tree-windows-evidence",
    [int]$TimeoutSeconds = 30
)
$ErrorActionPreference = 'Stop'
if ($TimeoutSeconds -lt 1) { throw 'TimeoutSeconds must be positive' }
New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
$LogDirectory = (Resolve-Path $LogDirectory).Path
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$stdout = Join-Path $LogDirectory "$stamp.stdout.log"
$stderr = Join-Path $LogDirectory "$stamp.stderr.log"
$summary = Join-Path $LogDirectory "$stamp.result.txt"
$exe = (Resolve-Path $Probe).Path
"Date=$(Get-Date -Format o)`nExecutable=$exe`nSHA256=$((Get-FileHash $exe -Algorithm SHA256).Hash)`nOS=$([Environment]::OSVersion)`nPowerShell=$($PSVersionTable.PSVersion)" | Set-Content -Encoding UTF8 $summary
$p = Start-Process -FilePath $exe -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
if (-not $p.WaitForExit($TimeoutSeconds * 1000)) {
    $p.Kill()
    $p.WaitForExit()
    'FAIL watchdog timeout; native evidence incomplete' | Add-Content $summary
    Get-Content $summary
    exit 124
}
$p.WaitForExit()
$p.Refresh()
$code = $p.ExitCode
"ExitCode=$code" | Add-Content $summary
Get-Content $stdout
Get-Content $stderr
Get-Content $summary
if ($code -ne 0) { exit $code }
if (-not (Select-String -Path $stdout -SimpleMatch 'PASS: ALL native dynamic-tree Windows assertions' -Quiet)) { exit 1 }
exit 0
