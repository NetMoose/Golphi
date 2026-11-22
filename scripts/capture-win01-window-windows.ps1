# Windows PowerShell 5.1. UTF-8 with BOM.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidateSet(100,150)][int]$Scale,
    [Parameter(Mandatory=$true)][string]$Exe,
    [Parameter(Mandatory=$true)][ValidateSet('hello-after','appearance')][string]$CaptureName,
    [string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'
$scriptDirectory = Split-Path -LiteralPath $MyInvocation.MyCommand.Path
if (!$PSBoundParameters.ContainsKey('OutputDirectory')) {
    $OutputDirectory = Join-Path $scriptDirectory '../artifacts/win01-font-evidence'
}
if (![Environment]::Is64BitProcess) { throw 'Нужен 64-битный PowerShell.' }
$Exe = (Resolve-Path -LiteralPath $Exe).Path
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$OutputDirectory = (Resolve-Path -LiteralPath $OutputDirectory).Path
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$log = Join-Path $OutputDirectory "$CaptureName-$Scale-$stamp.txt"
$shot = Join-Path $OutputDirectory "$CaptureName-$Scale-$stamp.png"
function Write-Log([string]$Text) {
    $Text | Out-File -LiteralPath $log -Encoding utf8 -Append
    Write-Host $Text
}
if (!("GolphiWin01Capture" -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class GolphiWin01Capture {
    [StructLayout(LayoutKind.Sequential)]
    public struct Rect { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] public static extern uint GetDpiForWindow(IntPtr hwnd);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
}
'@
}
Add-Type -AssemblyName System.Drawing
$os = Get-CimInstance Win32_OperatingSystem
Write-Log "OS=$($os.Caption); Version=$($os.Version); Build=$($os.BuildNumber); Architecture=$($os.OSArchitecture)"
Write-Log "PowerShell=$($PSVersionTable.PSVersion); RequestedScale=$Scale; EXE=$Exe"
Write-Log "SHA256=$((Get-FileHash -LiteralPath $Exe -Algorithm SHA256).Hash)"
$metadata = Join-Path (Split-Path -Parent $Exe) 'build.txt'
if (Test-Path -LiteralPath $metadata) { Get-Content -LiteralPath $metadata | ForEach-Object { Write-Log $_ } }
if (Test-Path -LiteralPath ($Exe + '.manifest')) { throw 'Внешний манифест не допускается: используется встроенный.' }
$process = Start-Process -FilePath $Exe -PassThru
try {
    if (!$process.WaitForInputIdle(15000)) { throw 'Окно не готово за 15 секунд.' }
    $hwnd = [IntPtr]::Zero
    for ($i=0; $i -lt 50; $i++) {
        $process.Refresh()
        if ($process.HasExited) { throw "Приложение завершилось до проверки: $($process.ExitCode)" }
        $hwnd = $process.MainWindowHandle
        if ($hwnd -ne [IntPtr]::Zero) { break }
        Start-Sleep -Milliseconds 100
    }
    if ($hwnd -eq [IntPtr]::Zero) { throw 'Главное окно не найдено.' }
    $dpi = [GolphiWin01Capture]::GetDpiForWindow($hwnd)
    $expected = [int](96 * $Scale / 100)
    Write-Log "WindowDPI=$dpi; ExpectedDPI=$expected"
    if ($dpi -ne $expected) { throw 'DPI окна не соответствует масштабу. Завершите приложение, обновите сеанс Windows и повторите запуск.' }
    if ($CaptureName -eq 'hello-after') {
        Write-Host 'Оставьте исходный текст «Мир» для сравнения с baseline на том же масштабе.'
    } else {
        Write-Host 'Выполните ручные пункты docs/windows-appearance-example.md: текст, clipping, ввод и добавление/скрытие/возврат подписи.'
    }
    Write-Host 'Уберите перекрывающие окна с формы перед снимком.'
    Read-Host 'Нажмите Enter для снимка текущего оформления' | Out-Null
    $previous = [GolphiWin01Capture]::SetThreadDpiAwarenessContext([IntPtr](-2))
    if ($previous -eq [IntPtr]::Zero) { throw 'Не удалось установить DPI-контекст для снимка.' }
    try {
        $rect = New-Object GolphiWin01Capture+Rect
        if (![GolphiWin01Capture]::GetWindowRect($hwnd, [ref]$rect)) { throw 'GetWindowRect не выполнен.' }
        $w = $rect.Right - $rect.Left
        $h = $rect.Bottom - $rect.Top
        if ($w -le 0 -or $h -le 0) { throw 'Некорректные размеры окна.' }
        $bitmap = New-Object System.Drawing.Bitmap -ArgumentList $w,$h
        try {
            $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
            try { $graphics.CopyFromScreen($rect.Left,$rect.Top,0,0,$bitmap.Size) }
            finally { $graphics.Dispose() }
            $bitmap.Save($shot, [System.Drawing.Imaging.ImageFormat]::Png)
        } finally { $bitmap.Dispose() }
        Write-Log "Screenshot=$shot; WindowRect=$($rect.Left),$($rect.Top),$w,$h"
    } finally { [void][GolphiWin01Capture]::SetThreadDpiAwarenessContext($previous) }
    if ($CaptureName -eq 'hello-after') {
        Write-Host 'После снимка проверьте ввод кириллицы, «Поздороваться» и обновление результата через 2 секунды.'
    }
    $result = Read-Host 'Введите PASS или FAIL для ручной проверки текущего окна'
    Write-Log "ManualWindow=$result"
    if ($result -notin @('PASS','FAIL')) { throw 'Нужен явный результат PASS либо FAIL.' }
    Read-Host 'Закройте форму обычным крестиком и нажмите Enter' | Out-Null
    if (!$process.WaitForExit(10000)) { throw 'Приложение не завершилось; закройте его вручную.' }
    Write-Log "ExitCode=$($process.ExitCode)"
    if ($result -ne 'PASS' -or $process.ExitCode -ne 0) { throw 'Ручная проверка либо завершение приложения дали ошибку.' }
    Write-Log 'WINDOW_CAPTURE=PASS; отдельные нативные проверки подтверждают HFONT/styles'
} catch {
    Write-Log "WINDOW_CAPTURE=FAIL: $_"
    throw
} finally {
    $process.Dispose()
}
