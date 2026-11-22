# Windows PowerShell 5.1. UTF-8 with BOM.
[CmdletBinding()]
param([string]$OutputDirectory = (Join-Path $PSScriptRoot '../artifacts/win01-resource-evidence'))
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$OutputDirectory = (Resolve-Path -LiteralPath $OutputDirectory).Path
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$log = Join-Path $OutputDirectory "resources-$stamp.txt"
function Write-Log([string]$Text) {
    $Text | Out-File -LiteralPath $log -Encoding utf8 -Append
    Write-Host $Text
}
function Invoke-Go([string[]]$Arguments) {
    Write-Log ('go ' + ($Arguments -join ' '))
    $commandPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $lines = & go @Arguments 2>&1
        $status = $LASTEXITCODE
    } finally { $ErrorActionPreference = $commandPreference }
    foreach ($line in $lines) { Write-Log ([string]$line) }
    if ($status -ne 0) { throw "go завершился с кодом $status" }
}
$names = @('CGO_ENABLED','GOOS','GOARCH','GOWORK','GOFLAGS','GOTOOLCHAIN','CC','CXX')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
try {
    if (![Environment]::Is64BitProcess) { throw 'Нужен 64-битный PowerShell.' }
    $os = Get-CimInstance Win32_OperatingSystem
    Write-Log "OS=$($os.Caption); Build=$($os.BuildNumber); Architecture=$($os.OSArchitecture); PowerShell=$($PSVersionTable.PSVersion)"
    $build = Join-Path $root 'artifacts/win01-stage1-build.txt'
    if (Test-Path -LiteralPath $build) { Get-Content -LiteralPath $build | ForEach-Object { Write-Log $_ } }
    $env:CGO_ENABLED='0'
    $env:GOOS='windows'
    $env:GOARCH='amd64'
    $env:GOWORK='off'
    $env:GOFLAGS='-mod=readonly'
    $env:GOTOOLCHAIN='local'
    $env:CC='golphi-no-c-compiler'
    $env:CXX='golphi-no-cxx-compiler'
    $version = & go version
    if ($LASTEXITCODE -ne 0 -or $version -ne 'go version go1.26.5 windows/amd64') { throw "Нужен Go 1.26.5 windows/amd64; получено: $version" }
    Invoke-Go -Arguments @('version')
    Invoke-Go -Arguments @('env','GOOS','GOARCH','CGO_ENABLED','GOTOOLCHAIN','CC','CXX')
    Invoke-Go -Arguments @('-C',(Join-Path $root 'tools/winres'),'list','-m','all')
    foreach ($example in @('hello','dynamic-tree')) {
        $app = Join-Path $root "examples/$example"
        $syso = Join-Path $app 'rsrc_windows_amd64.syso'
        $expected = (Get-FileHash -LiteralPath $syso -Algorithm SHA256).Hash
        Write-Log "ShippedResource=$example SHA256=$expected"
        $generated = Join-Path $OutputDirectory "$example-resource-$stamp"
        New-Item -ItemType Directory -Path $generated | Out-Null
        Invoke-Go -Arguments @('-C',(Join-Path $root 'tools/winres'),'tool','go-winres','make','--in',(Join-Path $app 'winres.json'),'--out',(Join-Path $generated 'rsrc'),'--arch','amd64')
        $actual = (Get-FileHash -LiteralPath (Join-Path $generated 'rsrc_windows_amd64.syso') -Algorithm SHA256).Hash
        if ($actual -ne $expected) { throw "Ресурс $example отличается после регенерации: $actual" }
        Write-Log "REGENERATE_$example=PASS; SHA256=$actual"
        $exe = Join-Path $OutputDirectory "$example-windows-amd64.exe"
        Invoke-Go -Arguments @('-C',$root,'build','-trimpath','-buildvcs=false','-ldflags=-H windowsgui','-o',$exe,"./examples/$example")
        Write-Log "EXE=$exe SHA256=$((Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash)"
        Invoke-Go -Arguments @('version','-m',$exe)
        $extracted = Join-Path $OutputDirectory "$example-manifest-$stamp"
        Invoke-Go -Arguments @('-C',(Join-Path $root 'tools/winres'),'tool','go-winres','extract','--xml-manifest','--dir',$extracted,$exe)
        $manifest = Join-Path $extracted 'app.manifest'
        $sourceManifest = Join-Path $app 'app.manifest.xml'
        if ((Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $sourceManifest -Algorithm SHA256).Hash) {
            throw "Встроенный XML $example отличается от исходного манифеста."
        }
        Get-Content -LiteralPath $manifest | ForEach-Object { Write-Log $_ }
        Write-Log "EMBEDDED_XML_$example=PASS"
    }
    Write-Log 'RESOURCE_BUILD=PASS; нативное оформление ещё требует запуска EXE и отдельного результата.'
    $hello = Join-Path $OutputDirectory 'hello-windows-amd64.exe'
    if (Test-Path -LiteralPath ($hello + '.manifest')) { throw 'Удалите внешний hello-windows-amd64.exe.manifest из каталога результатов и повторите проверку.' }
    Write-Host "Запустите $hello, проверьте ввод, кнопку и её тематизированный вид в сравнении с baseline; закройте форму."
    $result = Read-Host 'Введите PASS только если запуск, ввод и тематизированное оформление кнопки корректны; иначе FAIL'
    Write-Log "ManualThemedLaunch=$result"
    if ($result -ne 'PASS') { throw 'Запуск с манифестом не подтверждён.' }
} catch {
    Write-Log "RESOURCE_VERIFICATION=FAIL: $_"
    throw
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
