# Проверка динамического дерева на Windows

Это отдельный консольный эксперимент Win32. Production API Golphi он не меняет.
**Native-прогон 2026-10-29: PASS, Windows 11 build 26100, 96 DPI / 100%.** Журналы и ограничения диагностики — в [отчёте](dynamic-tree-validation.md#windows-native-прогон-2026-10-29). Сборка на Linux сама по себе не подтверждает работу Windows.

Нужна интерактивная Windows 11 amd64 с доступным рабочим столом. Во время короткого запуска не переключайте фокус: probe специально проверяет настоящий фокус EDIT. Запускайте обычным пользователем, не из фоновой службы. Временное окно закрывается автоматически. Watchdog завершает зависший процесс через 30 секунд.

Скопируйте рядом `probe-tree-windows.exe` и `verify-dynamic-tree-windows.ps1`, затем в PowerShell:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\verify-dynamic-tree-windows.ps1
$LASTEXITCODE
```

Если exe находится отдельно:

```powershell
.\verify-dynamic-tree-windows.ps1 -Probe C:\Temp\probe-tree-windows.exe -LogDirectory C:\Temp\golphi-evidence
```

Сохраните всю папку `dynamic-tree-windows-evidence`: stdout, stderr, результат с кодом выхода и SHA256. Успех: код 0 и последняя строка `PASS: ALL native dynamic-tree Windows assertions`. Код 124 означает watchdog, остальные ненулевые коды — ошибку. При ошибке нужен полный лог, а не только последняя строка.

В логе записываются версия Go, Windows build, startup/main thread, системный DPI и процент масштаба. Это system-aware DPI основного дисплея; для нескольких дисплеев запишите отдельно, на каком экране запускали, его масштаб и конфигурацию мониторов. Для согласованной матрицы нужен прогон при 100%. Probe открывает окно в координатах (80,80); во время короткой проверки не перемещайте его. Другие масштабы не требуются для этого этапа.

Probe проверяет одну форму, вложенные native STATIC-панели, отсутствие роста предков при oversized child, clipping через видимую область native DC (`PtVisible`), перенос EDIT между панелями, восстановление текста/фокуса/выделения/координат caret, пересоздание EDIT, ошибку SetParent на уничтоженный HWND и сохранение источника, вызов native EnumChildWindows callback на startup thread и отдельный worker thread. В тексте есть кириллица, emoji (суррогатная пара UTF-16), combining mark и японские символы; выделение 1..3 охватывает emoji. Отдельно проверяется свёрнутое выделение 4..4 (позиция ввода).

При пересоздании новый EDIT сначала создаётся и получает состояние, старый уничтожается после успешной проверки. Проверка ошибки использует реальную ошибку SetParent и явное восстановление состояния; это не проверка будущей production-транзакции. DC clipping не заменяет визуальную оценку артефактов перерисовки: при возможности приложите запись экрана. Проверка не покрывает IME/composition, reverse-selection direction, accessibility, пересечение разных форм/процессов/DPI awareness или пользовательский ввод во время переноса.

Для самостоятельной сборки из корня репозитория (Go 1.26.5):

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go build -o scripts/probe-tree-windows.exe ./cmd/probe-tree-windows
```

Probe намеренно собирается с консолью, без `-H=windowsgui`, чтобы сохранить диагностику. Обычные будущие GUI-примеры используют отдельный режим сборки.

Документация: [SetParent](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setparent), [EM_GETSEL](https://learn.microsoft.com/en-us/windows/win32/controls/em-getsel), [PtVisible](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-ptvisible). SetParent не меняет WS_CHILD автоматически; здесь все переносимые окна изначально имеют WS_CHILD и одинаковый DPI-контекст. Позиции EM_GETSEL задаются в TCHAR/UTF-16; используются полные DWORD, а не упакованное 16-битное возвращаемое значение.
