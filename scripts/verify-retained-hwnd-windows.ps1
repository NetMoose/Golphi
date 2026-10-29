param(
    [string]$BackendExe = "",
    [string]$UIExe = "",
    [string]$LogDirectory = "artifacts/retained-hwnd-windows",
    [switch]$Keyboard
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
    $timeout = if ($Name -eq 'TestNativeEditKeyboardSelectionProbe') { '210s' } else { '60s' }
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
Invoke-NativeTest $BackendExe 'TestNativeRetainedHWNDTransferWin32' 'backend-retained.log'
Invoke-NativeTest $UIExe 'TestNativeDynamicTreeIntegration' 'ui-native.log'
Write-Host "Обе автоматические нативные проверки PASS. Логи: $LogDirectory."
if ($Keyboard) {
    $previousProbe = $env:GOLPHI_EDIT_KEYBOARD_PROBE
    $previousTransaction = $env:GOLPHI_EDIT_KEYBOARD_TRANSACTION
    $previousRestore = $env:GOLPHI_EDIT_KEYBOARD_RESTORE_FOCUS
    try {
        $env:GOLPHI_EDIT_KEYBOARD_PROBE = '1'
        $env:GOLPHI_EDIT_KEYBOARD_TRANSACTION = '1'
        $env:GOLPHI_EDIT_KEYBOARD_RESTORE_FOCUS = '0'
        Write-Host 'Интеграционная проверка: щёлкните ABCDEF; End; стрелка влево; Shift+стрелка влево три раза; отпустите Shift; F8. Затем по подсказке формы Shift+стрелка вправо один раз, отпустите Shift, F9.'
        Invoke-NativeTest $BackendExe 'TestNativeEditKeyboardSelectionProbe' 'keyboard-transaction.log'
        $log = Get-Content -Raw (Join-Path $LogDirectory 'keyboard-transaction.log')
        if ($log -notmatch 'EDIT_KEYBOARD_SELECTION_PROBE_COMPLETE[^\r\n]*transaction=true') {
            throw 'Не подтверждён режим настоящей транзакции. Проверьте версию EXE.'
        }
    } finally {
        $env:GOLPHI_EDIT_KEYBOARD_PROBE = $previousProbe
        $env:GOLPHI_EDIT_KEYBOARD_TRANSACTION = $previousTransaction
        $env:GOLPHI_EDIT_KEYBOARD_RESTORE_FOCUS = $previousRestore
    }
} else {
    Write-Host 'Проверка настоящего обратного выделения клавиатурой НЕ запускалась. Для неё повторите с -Keyboard.'
}
Write-Host 'Это проверка переноса существующего HWND. Восстановление обратного выделения при пересоздании Edit ещё не решено; общая приёмка этапа остаётся открытой.'
