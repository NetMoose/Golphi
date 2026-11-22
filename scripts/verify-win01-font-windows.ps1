# Windows PowerShell 5.1. UTF-8 with BOM.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][ValidateSet(100,150)][int]$Scale,
    [string]$ArtifactDirectory,
    [string]$OutputDirectory
)
$ErrorActionPreference='Stop'
$scriptDirectory=Split-Path -LiteralPath $MyInvocation.MyCommand.Path
$root=(Resolve-Path -LiteralPath (Join-Path $scriptDirectory '..')).Path
if (!$PSBoundParameters.ContainsKey('ArtifactDirectory')) { $ArtifactDirectory=Join-Path $root 'artifacts/win01-font' }
if (!$PSBoundParameters.ContainsKey('OutputDirectory')) { $OutputDirectory=Join-Path $root 'artifacts/win01-font-evidence' }
$ArtifactDirectory=(Resolve-Path -LiteralPath $ArtifactDirectory).Path
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$OutputDirectory=(Resolve-Path -LiteralPath $OutputDirectory).Path
$stamp=Get-Date -Format 'yyyyMMdd-HHmmss'
$summary=Join-Path $OutputDirectory "font-$Scale-$stamp.txt"
function Write-Log([string]$Text) { $Text | Out-File -LiteralPath $summary -Encoding utf8 -Append; Write-Host $Text }
function Get-Artifact([string]$File) {
    $path=Join-Path $ArtifactDirectory $File
    if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw "Нет файла $path" }
    if (Test-Path -LiteralPath ($path+'.manifest')) { throw "Удалите внешний манифест $path.manifest" }
    Write-Log "ARTIFACT=$path; SHA256=$((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash)"
    return $path
}
function Invoke-Suite([string]$File,[string]$Regex,[string]$Name,[string[]]$Required) {
    $exe=Get-Artifact $File
    Write-Log "RUN=$File; Regex=$Regex"
    $log=Join-Path $OutputDirectory "$Name-$Scale-$stamp.log"
    $savedPreference=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & $exe '-test.v' "-test.run=$Regex" '-test.timeout=120s' 2>&1 | Tee-Object -FilePath $log
        $status=$LASTEXITCODE
    } finally { $ErrorActionPreference=$savedPreference }
    "EXIT_CODE=$status" | Add-Content -LiteralPath $log
    if ($null -eq $status -or $status -ne 0) { throw "$Name завершился с кодом $status" }
    $text=Get-Content -LiteralPath $log -Raw
    if ($text -match '--- SKIP:') { throw "$Name содержит SKIP" }
    foreach ($test in $Required) {
        $marker='(?m)^--- PASS: ' + [regex]::Escape($test) + '(?:\s|$)'
        if ($text -notmatch $marker) { throw "$Name не подтвердил обязательный тест $test" }
    }
    Write-Log "$Name=PASS; LOG=$log"
}
$variables=@('GOLPHI_WIN01_EXPECT_DPI','GOLPHI_WIN01_EXPECT_STYLES','GOLPHI_NATIVE_TREE')
$saved=@{}
foreach ($name in $variables) { $saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
try {
    if (![Environment]::Is64BitProcess) { throw 'Нужен 64-битный PowerShell.' }
    $os=Get-CimInstance Win32_OperatingSystem
    Write-Log "OS=$($os.Caption); Build=$($os.BuildNumber); Architecture=$($os.OSArchitecture); PowerShell=$($PSVersionTable.PSVersion)"
    $dpi=[int](96*$Scale/100)
    Write-Log "RequestedScale=$Scale; ExpectedDPI=$dpi"
    Get-Content -LiteralPath (Join-Path $ArtifactDirectory 'build.txt') | ForEach-Object { Write-Log $_ }
    $fontTests=@('TestWindowsFontABI','TestWindowsFontAtRequestedDPI','TestWindowsFontLifetime','TestWindowsFontStartupFaults','TestWindowsFontCleanupFaults','TestWindowsFontPreservesCallbackFailure','TestWindowsFontVisualStyles','TestWindowsFontEditCreationPublishesOriginalProc')
    $env:GOLPHI_WIN01_EXPECT_DPI=[string]$dpi
    $env:GOLPHI_WIN01_EXPECT_STYLES='0'
    Invoke-Suite -File 'golphi-win32-classic.test.exe' -Regex '^TestWindowsFont' -Name 'font-classic' -Required $fontTests
    $env:GOLPHI_WIN01_EXPECT_STYLES='1'
    Invoke-Suite -File 'golphi-win32-themed.test.exe' -Regex '^TestWindowsFont' -Name 'font-themed' -Required $fontTests
    $env:GOLPHI_NATIVE_TREE='1'
    Invoke-Suite -File 'golphi-win32-themed.test.exe' -Regex '^TestNative(DynamicTreeWin32|RetainedHWNDTransferWin32|AbortedTreeCleanupWin32)$' -Name 'backend-regression' -Required @('TestNativeDynamicTreeWin32','TestNativeRetainedHWNDTransferWin32','TestNativeAbortedTreeCleanupWin32')
    Invoke-Suite -File 'golphi-ui-themed.test.exe' -Regex '^(TestStartupFailure|TestNativeDynamicTreeIntegration)$' -Name 'ui-regression' -Required @('TestStartupFailure','TestNativeDynamicTreeIntegration')
    Write-Log 'AUTOMATED_CHECKS=PASS; ручные сценарии ещё не подтверждены.'
    $hello=Get-Artifact 'hello.exe'
    $appearance=Get-Artifact 'windows-appearance.exe'
    $tree=Get-Artifact 'dynamic-tree.exe'
    $capture=Join-Path $scriptDirectory 'capture-win01-window-windows.ps1'
    Write-Host "Сначала снимок $hello для сравнения с прежним baseline на том же DPI."
    & $capture -Scale $Scale -Exe $hello -CaptureName 'hello-after' -OutputDirectory $OutputDirectory
    Write-Host "Теперь $appearance: выполните docs/windows-appearance-example.md перед снимком."
    & $capture -Scale $Scale -Exe $appearance -CaptureName 'appearance' -OutputDirectory $OutputDirectory
    Write-Log 'ManualAppearance=PASS; два снимка и DPI окон записаны отдельными capture-журналами.'
    Write-Host "Запустите $tree для проверки прямого переноса поля."
    Write-Host 'Нажмите «Перенести через 8 с», верните фокус в поле; проверьте ввод без нового щелчка и прямое/обратное выделение.'
    Write-Host 'Отдельно повторите с выделением Shift+Home и Shift+End, включая выделение до конца строки; после переноса оно должно сохраниться.'
    $transfer=Read-Host 'Проверка ввода, фокуса и прямого переноса сохраняемого Edit: PASS или FAIL'
    Write-Log "ManualRetainedTransfer=$transfer"
    if ($transfer -ne 'PASS') { throw 'Перенос поля не подтверждён.' }
    Write-Log 'WINDOWS_FONT_RUN=PASS'
} catch {
    Write-Log "WINDOWS_FONT_RUN=FAIL: $_"
    throw
} finally {
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
