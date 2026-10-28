# Проверка startup thread на Windows

Проверка предназначена для **Windows 11/amd64 с обычным рабочим столом**. Программа создаёт одно скрытое нативное окно STATIC, получает вызовы Win32 `EnumWindows` (в том числе для своего окна), сравнивает идентификаторы системных потоков при запуске, в main и в callback, проверяет отдельный поток фоновой goroutine и уничтожает окно на исходном потоке. Нажимать что-либо не требуется.

Программа выводит фактическую сборку Windows и системный DPI/масштаб после включения режима учёта системного DPI. Этот масштаб относится к системе/основному монитору, а не ко всем мониторам.

## Запуск готовой программы

1. Скопируй `golphi-probe-windows-amd64.exe` на Windows.
2. Открой PowerShell в папке с этим файлом.
3. Выполни приведённые ниже команды целиком.
4. Пришли полученный файл `golphi-windows-probe.txt`, даже если проверка завершится ошибкой. В журнал попадёт и код завершения программы.

```powershell
$ErrorActionPreference = 'Stop'
$log = Join-Path $PWD 'golphi-windows-probe.txt'
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$os = Get-CimInstance Win32_OperatingSystem
"Host CPU architecture code: $($cpu.Architecture) (9=AMD64, 12=ARM64); OS: $($os.Caption); build: $($os.BuildNumber); OS architecture: $($os.OSArchitecture)" | Tee-Object -FilePath $log
& .\golphi-probe-windows-amd64.exe 2>&1 | Tee-Object -FilePath $log -Append
$probeExit = $LASTEXITCODE
"Probe exit code: $probeExit" | Tee-Object -FilePath $log -Append
if ($probeExit -ne 0) { throw "Native probe failed (exit $probeExit); preserve $log" }
```

## Как выглядит успешный результат

Проверка должна вывести:

- `PASS` и код завершения `0`;
- число callback больше нуля и `own-window-seen=true` — собственное окно найдено;
- одинаковые идентификаторы потоков startup/main/callback/cleanup;
- другой идентификатор потока worker;
- `DestroyWindow=1` — окно успешно уничтожено.

Сохрани полный журнал и при неудаче. Сохрани сведения о названии ОС и архитектуре: Windows 11 сообщает внутреннюю основную версию 10, поэтому строка version=10.0 сама по себе не означает Windows 10. Запускай программу из обычного сеанса рабочего стола, а не из службы или сеанса без графического интерфейса. Для запроса DPI требуется Windows 10 версии 1607 или новее; если API недоступен, программа сообщит об ошибке.

## Самостоятельная сборка, если потребуется

Для запуска готового `.exe` устанавливать Go или компилятор C не нужно. Чтобы самостоятельно собрать файл **на Linux из корня репозитория**, используй Go 1.26.5:

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/golphi-probe-windows-amd64.exe ./cmd/probe-windows
```

## Что проверяет этот эксперимент

Это узкая проверка нативных вызовов, callback и потоков. Она ещё не реализует `Form.Run`, цикл обработки сообщений Win32, передачу задач из Go в UI, прикладные события или полный порядок завершения приложения. Успешные переносимые unit-тесты и кросс-компиляция **не подтверждают** корректность работы на Windows. Задача 1.4 выполнена по журналу пользователя на Windows 11 Pro/amd64 build 26100, масштаб 100%; результаты зафиксированы в [отчёте проверки](mvp-validation.md). Windows 10 этим запуском не проверена.

Документация: [EnumWindows](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumwindows), [CreateWindowExW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-createwindowexw), [GetDpiForSystem](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdpiforsystem). Сначала была предпринята попытка обращения к Context7, но она завершилась транспортной ошибкой; вместо него использованы эти официальные материалы Microsoft.
