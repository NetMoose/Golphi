# WIN-01: ход реализации

## Ресурсный этап, 22 ноября 2026

Изменение `modernize-win32-appearance`, ветка `codex/modernize-win32-appearance`.
База develop: `5cfdcb256e12a589e42f5ad1e5605e3c6cc8bf85`; исходный код
baseline: `acc3afe51784cd1f75a4de248260ba442504aad1`.

Выполнены подготовка go-winres v0.3.3 в отдельном tool-модуле, явные XML/JSON
и `.syso` hello/dynamic-tree, Linux-генератор, инструкция для собственного
манифеста. Добавлены LF-атрибуты исходников ресурсов и бинарный атрибут `.syso`.
Корневые go.mod/go.sum, библиотечный API, Win32 backend и GTK backend не изменены.
Baseline GUI EXE собран **до** добавления манифеста; его ресурсная таблица пуста.
Команда и SHA256 — [build.txt](../artifacts/win01-baseline/build.txt).

Подготовлены русские baseline/resource Windows-скрипты и
[инструкция первого прогона](win01-windows-stage1.md). Оба русских `.ps1` имеют
UTF-8+BOM. Скрипты проверены статически; Windows PowerShell 5.1 в Linux-среде
не исполнялся. На тот момент фактического Windows-прогона ещё не было; результаты его
последующего выполнения приведены ниже.

## Проверки на Linux

Go 1.26.5 linux/amd64, CGO_ENABLED=0:

- go-winres module verify, sh -n, повторная генерация без сети — PASS;
- оба примера собраны Linux/amd64 и Windows/amd64 без C toolchain,
  Windows GUI subsystem, Linux не включает `.syso` — PASS;
- в каждом Windows PE один RT_MANIFEST #1 / 0409; извлечённый XML
  побайтно совпадает с исходником — PASS;
- Common Controls v6, system-aware DPI, asInvoker/uiAccess=false — PASS;
- отдельный XML с longPathAware после генерации/сборки/извлечения сохранён — PASS;
- go test ./..., go vet ./..., Windows-target go vet ./... — PASS;
- strict OpenSpec change validation, локальные ссылки и BOM — PASS.

Закреплены зависимости в [tools/winres](../tools/winres/go.mod) и его go.sum.
Контрольные суммы ресурсов:

```text
ab2527b9b038754046dd3651c773f5267a40f7bebb30911a263e493ca4ce6410  examples/hello/rsrc_windows_amd64.syso
4ec5c3819646dd0e782b773aeab7953033e0e46fee34a721c6fcc5c837f78929  examples/dynamic-tree/rsrc_windows_amd64.syso
```

Независимое review ресурсного этапа пройдено. В Windows-verifier исправлены
изменение исходного `.syso` при сравнении, отсутствие проверки sidecar-файла
и недостаточно явный критерий тематизированного вида кнопки. Повторное review
подтвердило исправления. Теперь генерация не меняет исходные ресурсы.

## Состояние перед Windows-прогоном

- Задача 1.1: снимки и ручной результат baseline при фактических DPI 96 и 144.
- Задача 1.4: фактическая генерация и сборка на Windows с CGO_ENABLED=0,
  совпадение ресурсов, извлечение XML и запуск без sidecar-манифеста.

Кросс-сборка не является native acceptance. Реализация системного шрифта,
его lifetime/failure tests и итоговый проверочный пример ещё не начаты.
После baseline продолжается шрифтовой этап; окончательные Windows- и Linux-native
прогоны выполняются по задачам 4.1–4.2. Общие открытые вопросы и DOC-01 сохранены.

Ресурсный checkpoint `b782df356a5749c91714247879f244d49f76af25` отправлен в origin; HEAD совпал.
Задачи 1.2, 1.3 и 1.5 завершены (3/18). Ожидаются Windows 1.1/1.4.

## Исправление старта Windows-скриптов

Первый запуск baseline на Windows завершился до создания окна:
`Join-Path` получил пустой `$PSScriptRoot` при вычислении параметра по умолчанию.
[Исходный журнал ошибки](../artifacts/win01-baseline-startup-failure.txt) сохранён
как FAIL, а не приёмка Golphi. Похожий ранний default был и в resource-verifier.
Пути обоих скриптов перенесены в тело после param; явные параметры сохраняются.
Пакет v2 содержит тот же baseline EXE, новый шрифт не добавлен.
На момент выдачи v2 повторный запуск Windows PowerShell 5.1 ещё ожидался.
Последующее подтверждение — в разделе ниже.

## Подтверждённый Windows этап 1, 22 ноября 2026

[Evidence](../artifacts/win01-stage1-windows-20261122/): Windows 11 Pro
10.0.26100 build 26100, amd64; PowerShell 5.1.26100.7462; Go 1.26.5 windows/amd64.
Оригиналы оставлены без изменений в пользовательском Windows-каталоге. Копии
текстов нормализованы в UTF-8/LF без trailing whitespace, PNG скопированы
побайтно; SHA256SUMS относится к сохранённым копиям. Console Log.txt дублирует
итоговый resource-прогон, не считается вторым независимым запуском.

| Проверка | Результат |
|---|---|
| Baseline 100%, фактический DPI окна 96 | ManualBaseline=PASS, ExitCode=0, PNG 416×219 |
| Baseline 150%, фактический DPI окна 144 | ManualBaseline=PASS, ExitCode=0, PNG 622×326 |
| Исходный baseline EXE | SHA256 совпал с build.txt и обоими журналами |
| Генерация hello/dynamic-tree на Windows | PASS, оба `.syso` побайтно совпали с ресурсами репозитория |
| GUI-сборки hello/dynamic-tree на Windows | PASS, CGO_ENABLED=0; CC/CXX заданы несуществующими командами |
| Встроенные манифесты обоих EXE | Извлечены, XML побайтно совпал с исходным, Common Controls v6 и system-aware сохранены |
| hello без sidecar-манифеста | ManualThemedLaunch=PASS; на снимке видна тематизированная кнопка и результат обработчика |
| Исправленные PowerShell-скрипты | Оба сценария фактически завершены после исправления путей |

Проверены также реальные файлы EXE в каталоге результатов: их SHA256 совпали
с журналом, а не только с метаданными исходного пакета:

```text
46830f787516106378660cca220303cb8e5c4ffca76165c152e5661504021465  hello-windows-amd64.exe
94f11e317397a5b2f74c0400cd1b2ad93033ae06a3de08f952bd80a3e1b1a1f6  dynamic-tree-windows-amd64.exe
```

Задачи 1.1 и 1.4 завершены; весь этап 1 — 5/5, общий прогресс — 5/18.
Метаданные пакета содержат историческое PENDING; результат определяется
последующими реальными действиями и итоговыми маркерами журналов, а не этой
строкой. На снимке тематизированного hello масштаб/DPI отдельно не записан;
он подтверждает ручной ресурсный запуск, а не приёмку нового шрифта на обоих
масштабах. Текущий системный шрифт ещё не реализован. Автоматическая проверка
активации styles/readback HFONT и окончательные регрессии остаются в 3.2/4.1–4.2.
Позиции пересозданного Edit и остальные открытые вопросы не закрываются.
