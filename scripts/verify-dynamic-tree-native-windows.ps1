param(
    [string]$BackendExe = "",
    [string]$UIExe = "",
    [string]$LogDirectory = "artifacts/dynamic-tree-windows",
    [switch]$Manual
)
$ErrorActionPreference = 'Stop'
if ([Environment]::OSVersion.Platform -ne 'Win32NT') { throw 'Нужен Windows 11 amd64 с интерактивным рабочим столом.' }
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
$LogDirectory = (Resolve-Path $LogDirectory).Path
$env:CGO_ENABLED = '0'
$env:GOLPHI_NATIVE_TREE = '1'
if (-not $BackendExe) {
    $BackendExe = Join-Path $LogDirectory 'golphi-win32.test.exe'
    & go test -c -o $BackendExe ./internal/backend/win32
    if ($LASTEXITCODE -ne 0) { throw "Сборка backend: код $LASTEXITCODE" }
}
if (-not $UIExe) {
    $UIExe = Join-Path $LogDirectory 'golphi-ui.test.exe'
    & go test -c -o $UIExe ./ui
    if ($LASTEXITCODE -ne 0) { throw "Сборка UI: код $LASTEXITCODE" }
}
$BackendExe = (Resolve-Path $BackendExe).Path
$UIExe = (Resolve-Path $UIExe).Path
@(
    "UTC=$([DateTime]::UtcNow.ToString('o'))"
    "OS=$([Environment]::OSVersion.VersionString)"
    "64-bit process=$([Environment]::Is64BitProcess)"
    'Масштаб/DPI и версия Windows 11 должны быть дополнительно записаны оператором.'
    "Backend SHA256=$((Get-FileHash $BackendExe -Algorithm SHA256).Hash)"
    "UI SHA256=$((Get-FileHash $UIExe -Algorithm SHA256).Hash)"
) | Set-Content -Encoding UTF8 (Join-Path $LogDirectory 'environment.txt')
function Invoke-NativeTest([string]$Executable, [string]$Name, [string]$LogName) {
    $path = Join-Path $LogDirectory $LogName
    $timeout = if ($Name -eq 'TestNativeDynamicTreeManual') { '210s' } else { '60s' }
    # Прямой вызов: LASTEXITCODE относится к завершившемуся EXE, не к Start-Process.
    $savedPreference = $ErrorActionPreference
    try {
        # Windows PowerShell wraps native stderr in ErrorRecord; retain it in the log.
        $ErrorActionPreference = 'Continue'
        & $Executable '-test.v' "-test.run=^$Name`$" "-test.timeout=$timeout" 2>&1 | Tee-Object -FilePath $path
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $savedPreference
    }
    "EXIT_CODE=$code" | Add-Content $path
    $log = Get-Content -Raw $path
    if ($null -eq $code -or $code -ne 0) { throw "$Name завершился с кодом $code" }
    if ($log -match '--- SKIP:' -or $log -notmatch "--- PASS: $Name(?:\s|$)") {
        throw "${Name}: нет подтверждённого PASS, либо есть SKIP; нативная проверка не принята."
    }
}
Invoke-NativeTest $BackendExe 'TestNativeDynamicTreeWin32' 'backend-native.log'
Invoke-NativeTest $UIExe 'TestNativeDynamicTreeIntegration' 'ui-native.log'
Write-Host "Обе автоматические нативные проверки PASS. Логи: $LogDirectory."
if ($Manual) {
    $env:GOLPHI_NATIVE_TREE_MANUAL = '1'
    Write-Host 'Отдельная ручная проверка: 180 секунд. Команды вводятся в эту консоль; нужен PID внешнего Notepad.'
    Invoke-NativeTest $BackendExe 'TestNativeDynamicTreeManual' 'manual-operator.log'
    Write-Host 'Ручной сценарий завершён с подтверждениями оператора. Это отдельное свидетельство; приложите снимок clipping.'
} else {
    Write-Host 'Ручной сценарий НЕ запускался. Для него повторите с -Manual; инструкция и команды: DETAILS-RU.md в архиве или docs/dynamic-tree-win32-integration.md в репозитории.'
}
