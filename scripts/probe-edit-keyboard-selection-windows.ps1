param(
    [string]$BackendExe = "./golphi-win32.test.exe",
    [string]$LogDirectory = "artifacts/edit-keyboard-selection-probe"
)
$ErrorActionPreference = 'Stop'
if ([Environment]::OSVersion.Platform -ne 'Win32NT') { throw 'Нужна Windows с интерактивным рабочим столом.' }
Set-Location (Split-Path -Parent $PSScriptRoot)
$BackendExe = (Resolve-Path $BackendExe).Path
New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
$LogDirectory = (Resolve-Path $LogDirectory).Path
@(
    "UTC=$([DateTime]::UtcNow.ToString('o'))"
    "OS=$([Environment]::OSVersion.VersionString)"
    "64-bit process=$([Environment]::Is64BitProcess)"
    "Backend SHA256=$((Get-FileHash $BackendExe -Algorithm SHA256).Hash)"
) | Set-Content -Encoding UTF8 (Join-Path $LogDirectory 'environment.txt')
$previousProbe = $env:GOLPHI_EDIT_KEYBOARD_PROBE
$previousPreference = $ErrorActionPreference
try {
    $env:GOLPHI_EDIT_KEYBOARD_PROBE = '1'
    Write-Host 'Щёлкните Edit: End, один раз стрелка влево без Shift, затем три раза Shift+стрелка влево. Отпустите Shift и нажмите F8. Следуйте подсказкам окна; затем один раз Shift+стрелка вправо и F9.'
    $ErrorActionPreference = 'Continue'
    & $BackendExe '-test.v' '-test.run=^TestNativeEditKeyboardSelectionProbe$' '-test.timeout=210s' 2>&1 | Tee-Object -FilePath (Join-Path $LogDirectory 'keyboard-selection-probe.log')
    $code = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $previousPreference
    $env:GOLPHI_EDIT_KEYBOARD_PROBE = $previousProbe
}
"EXIT_CODE=$code" | Add-Content (Join-Path $LogDirectory 'keyboard-selection-probe.log')
$log = Get-Content -Raw (Join-Path $LogDirectory 'keyboard-selection-probe.log')
if ($null -eq $code -or $code -ne 0) { throw "Диагностика завершилась с кодом $code. Передайте весь каталог $LogDirectory." }
if ($log -match '--- SKIP:' -or $log -notmatch '--- PASS: TestNativeEditKeyboardSelectionProbe(?:\s|$)' -or $log -notmatch 'EDIT_KEYBOARD_SELECTION_PROBE_COMPLETE') {
    throw "Диагностика не выполнена полностью. Передайте весь каталог $LogDirectory."
}
Write-Host "Диагностика завершена. Это НЕ подтверждение исправления или приёмки. Передайте весь каталог $LogDirectory."
