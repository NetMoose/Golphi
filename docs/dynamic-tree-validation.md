# Проверка динамического дерева компонентов

Изменение: [implement-dynamic-component-tree](../openspec/changes/implement-dynamic-component-tree/proposal.md). Реализация начата; результаты этого отчёта не означают завершения изменения.

## Исходная точка

Ветка `codex/dynamic-component-tree` создана от актуальной `develop`, коммит `82605a8`, без worktree. Исходные отслеживаемые файлы чистые; единственные подготовленные файлы — согласованное предложение. Базовая команда `CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go test -count=1 ./...` прошла. Этот базовый запуск без native opt-in не заменяет нативные проверки.

Сохраняются Go 1.26.5/purego v0.11.2 и матрица принятого MVP: Linux Mint 22.3 amd64 X11 GTK4; Windows 11 Pro amd64 build 26100, масштаб 100%/96 DPI. Фактические версии и масштаб каждого нового прогона фиксируются отдельно.

## Технический этап

| Проверка | Статус |
|---|---|
| Подготовка feature-ветки и базовая регрессия | PASS |
| Linux: вложенность/clipping, перенос и состояние ввода | PASS в ограниченном техническом эксперименте; [журнал](../artifacts/dynamic-tree-linux-probe.txt), [условия и ограничения](dynamic-tree-linux-probe.md) |
| Windows: подготовка executable и инструкции | Cross-build и review PASS; [русская инструкция](dynamic-tree-windows-probe.md) |
| Windows: фактический запуск нового native-примера | PASS, 2026-10-29, build 26100, 96 DPI / 100% |
| Общий Driver для динамического дерева | Внутренний протокол и общий координатор реализованы; fake tests и cross-build PASS, native-интеграция в задачах 4.x |

Технические пробы не являются реализацией публичного динамического API. Ранее принятые MVP-журналы не доказывают новых свойств дерева. Cross-build отдельно от Windows native-прогона. Остальные открытые вопросы сохранены; основные specs не синхронизируются до отдельного запроса.

## Свидетельства первого checkpoint

2026-10-28: Linux Mint 22.3 amd64, Go 1.26.5, пакет GTK4 `4.14.5+ds-0ubuntu0.10`, X11 `DISPLAY=:0`, `GDK_BACKEND=x11`. Переменные GDK_SCALE/GDK_DPI_SCALE не заданы; отдельная матрица масштабов не проверялась. Журнал получен командой:

```sh
GOLPHI_TREE_PROBE=1 CGO_ENABLED=0 GDK_BACKEND=x11 DISPLAY=:0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go test -count=1 -v ./cmd/probe-tree-linux
```

Нативный эксперимент подтверждает вложенность без роста от ребёнка 1200×600, clipping через hit-test и отрисовку GTK (1200 изменённых пикселей внутри, 0 снаружи), перенос и пересоздание Edit с сохранением фокуса/Unicode-позиций, отсутствие auto-scroll при фокусе, disconnect и финализацию двух Edit. Это не полная интеграция динамического API. Экспериментальная пауза для смены фокуса не принимается как production-протокол: следующий этап должен найти событийное решение с сохранением синхронного контракта SetParent либо сообщить blocker.

Windows-проба проверена ревью и собрана с `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`. На момент первого checkpoint фактический Windows-прогон ещё ожидался; его результат приведён ниже. На том checkpoint технический этап 1.5 ещё не был завершён.

Финальные проверки checkpoint: `CGO_ENABLED=0 go test -count=1 ./...`, `go vet ./...` с теми же offline caches; Windows cross-build и target vet для `cmd/probe-tree-windows`; strict validation изменения и всех пяти основных specs. Общий go test без opt-in не запускает native-тесты.

## Windows native-прогон 2026-10-29

Пользователь выполнил запуск на Windows 11 amd64 build 26100, Go 1.26.5, 96 DPI / 100%, PowerShell 5.1.26100.7462. Сохранены исходные [stdout](../artifacts/dynamic-tree-windows-evidence/20261029-024743.stdout.log), [stderr](../artifacts/dynamic-tree-windows-evidence/20261029-024743.stderr.log) и [result](../artifacts/dynamic-tree-windows-evidence/20261029-024743.result.txt) без изменений.

SHA256 executable `8117275b59becff7bb47babeba433206d2543c3efc072eb31b6e1564bff11880` совпадает с переданным архивом, построенным из `fe006c4`. Финальная строка stdout — `PASS: ALL native dynamic-tree Windows assertions`, stderr пуст. Пользователь сообщил `$LASTEXITCODE = 0`. Поле `ExitCode` в result пустое: журнал runner не доказывает числовой код самого дочернего процесса; это ограничение диагностики сохраняется явно, исходный журнал не исправлялся.

PASS: вложенные панели без роста предков, DC clipping, прямой перенос Edit, пересоздание с сохранением текста/фокуса/UTF-16 selection и caret, реальная ошибка SetParent на недействительный HWND с сохранением источника и восстановлением состояния, native callback на startup thread, отдельный worker thread и уничтожение формы. Сообщение `FAILURE-INJECTION ... Invalid window handle` — ожидаемый проверяемый отказ, после которого assertions проходят.

Задача 1.3 завершена. Доступен следующий шаг 1.4 — внутренний Driver на основании обоих экспериментов. Это ещё не проверка production-транзакций или всего динамического API; ограничения технических проб и остальные открытые вопросы сохраняются.

## Внутренний Driver — 2026-10-29

Добавлены Panel, запрос/результат операции над полным поддеревом, снимок ввода в Unicode scalar offsets, поколения маршрутизации событий и протокол Prepare/Commit/Rollback/Cleanup/Finish. [Контракт backend](backend-contract.md) определяет владение попыткой и старым состоянием, публикацию под запретом доставки, поздние сигналы и безопасное освобождение callback roots.

Общий `ApplyTree` проверен fake native-операциями: публикация целого результата после commit, отсутствие публикации при ошибках Prepare/Commit, rollback, аварийный hook до cleanup, сохранение исходной ошибки и сбоев восстановления/очистки, перехват cleanup panic со стеком и продолжение независимой очистки. Эти тесты проверяют реальную общую логику, но не поведение toolkit.

GTK/Win32 временно используют явный `ErrTreeUnsupported` для новых методов. Прежний статический путь и examples/hello сохраняются. Публичный runtime ещё не вызывает динамическую границу; это не завершённая реализация Panel/SetParent во время Run. Настоящие транзакции, преобразование native-позиций, поколения сигналов и безопасное завершение callback будут проверяться в последующих задачах. В частности, синхронное восстановление фокуса GTK без экспериментальной паузы остаётся обязательной проверкой реализации, а не уже доказанной возможностью адаптера.

Проверки после изменения: `CGO_ENABLED=0 go test -count=1 ./...`, Linux и Windows `go vet ./...`, `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`, strict validation изменения и пяти основных specs, `git diff --check` — PASS. Использованы offline caches `/tmp/golphi-gocache` и `/tmp/golphi-gomodcache`, `GOPROXY=off`. Native-пробы в этом checkpoint повторно не запускались: их код и нативный статический путь не менялись. Результаты прежних прогонов не объявляются проверкой новой production-интеграции.

Независимое review задач 1.4–1.5: соответствие требованиям и качество — PASS, блокирующих замечаний нет. Технический этап завершён; задачи логического графа, runtime-транзакций и нативной интеграции остаются открытыми.

## Логический граф и guards — 2026-10-29

Добавлены экспериментальные закрытые роли `Component`/`Container` и логическая `Panel` без текстового API. Owner обязателен и неизменяем, кроме корневой Form; nil/typed-nil и уничтоженный Owner отклоняются до регистрации. Parent допускает Form/Panel одной корневой формы по цепочке Owner; self-parent, циклы, другой корень и уничтоженная цель дают error без изменения связей и уведомлений. Уничтоженный получатель проверяется первым.

Startup обходит общие Owner/Parent связи, создаёт визуальное дерево в порядке родитель перед ребёнком и учитывает отсоединённых owned-потомков. Тест с fake Driver проверяет вложенные панели и обратный порядок создания логических объектов. Каскад из callback ребёнка проверяет немедленный Destroyed, пересекающиеся связи, подавление позднего Click, освобождение после callback ровно один раз и сохранение работоспособности соседнего компонента.

Во время Run обычные конструкторы и связывание отсоединённых объектов доступны на UI-потоке без native-вызовов. SetBounds разрешён живому отсоединённому поддереву независимо от прежнего started; geometry присоединённых started-компонентов и SetClientSize после начала Run остаются запрещены. Worker-вызовы конструкторов, SetParent и SetBounds, включая no-op, вызывают panic на worker до изменений; subprocess проверяет перехват там же и положительный путь через Post от worker. Новые detached-компоненты пока не получают runtime/native binding: это часть следующего этапа материализации.

Проверки: полный `CGO_ENABLED=0 go test -count=1 ./...` (включая subprocess/fake сценарии), `go vet ./...` для Linux и Windows, `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`, strict validation изменения и пяти основных specs, `git diff --check` — PASS. Кеши `/tmp/golphi-gocache`, `/tmp/golphi-gomodcache`, `GOPROXY=off`.

Этот этап не подтверждает отображение Panel в GTK/Win32, clipping или production-перенос во время Run. Нативные адаптеры ещё не реализуют новые операции; реальные изменения присоединённого дерева во время Run пока явно отклоняются. Снимки ввода, атомарная материализация, смена базы/отмена уведомлений по всему поддереву и обработка ошибок транзакций остаются задачами 3.x/4.x. Несколько окон, смена Owner и остальные открытые вопросы не закрывались.

Итоговое review логического этапа: соответствие требованиям и качество — PASS, блокирующих дефектов нет. Уточнены документация startup-порядка и граница принятого native MVP относительно логического API.

## Транзакции, события и отказы общего ядра — 2026-10-29

Этот этап дополняет предыдущий логический граф: публичный SetParent подключён к ApplyTree, новые компоненты учитываются runtime, а peer-связи публикуются с поколениями маршрутизации. GTK/Win32 пока сохраняют UnsupportedTree; результаты ниже относятся к общему ядру с fake Driver, не к завершённой native-интеграции.

| Гарантия | Свидетельство |
|---|---|
| Готовность всего поддерева до nil, обычный отказ позднего ребёнка и повторная попытка | TestTreeMaterializationRollbackRetry |
| Четыре перехода, подавление технических сигналов, сохранение/отмена pending и однократный teardown | TestTreeAttachmentTransitions |
| Новая база OnChange после reattach, отсутствие рекурсии, nil/replacement/destroy и устаревшие поколения | TestTreeReattachmentComparison (8 сценариев) |
| Unicode-позиции и ввод, fresh native state, прямой перенос/откат, detached same/changed text, без захвата фокуса | TestTreeInputState |
| E/E1/W при восстановленном состоянии: отдельная диагностика и продолжение работы | TestTreeRecoverableCleanup |
| E/R/cleanup при невозможном восстановлении, в том числе первом attach, остановка до cleanup, завершение callback и гонка Post | TestTreeFatalRollback (2 сценария) |

Тесты используют настоящий Form.Run/SetParent и общий координатор; fake моделирует native-размещение, ввод, ошибки и безопасное отложенное освобождение. Прикладные события, очереди, публикация, снимок отсоединённого Edit и остановка остаются production-кодом. Config.Input передаёт подготовленное выделение при первом запуске; реальное применение этого поля native-адаптерами требуется на этапе 4.x.

Регрессия старых callback/Post panic, normal close и startup failure проходит в общем наборе. Также проверены CGO_ENABLED=0 сборки Linux и Windows amd64, vet обеих целей, strict validation изменения и пяти основных specs, git diff --check. Проверка Windows здесь — сборка; нового нативного Windows-прогона этот этап не заявляет. Точные правила и ограничения описаны в [runtime](dynamic-tree-runtime.md), [событиях](contracts/component-events.md) и [диагностике попытки](dynamic-tree-failures.md).

Сохранены открытыми публичные диагностические типы/формат, сочетания fatal SetParent с последующими вызовами/паникой обработчика, retry и судьба неосвобождённых ресурсов, порядок зависимой очистки и остальные исключённые вопросы. Реальный синхронный GTK focus protocol, clipping, преобразование native-позиций, late callbacks и callback roots должны быть подтверждены адаптерами отдельно.

Итоговое review этапа 3: соответствие спецификации и качество — PASS, блокирующих замечаний нет. Дополнительная проверка `CGO_ENABLED=1 go test -race ./ui ./internal/backend` — PASS. Все перечисленные проверки выполнены координатором после объединения задач этапа; нативная интеграция 4.x остаётся незавершённой.

## Нативная интеграция — 2026-10-29

GTK4 и Win32 реализуют Panel, Config.Input, SnapshotInput и транзакции дерева. GTK переносит поддеревья с Edit через пересоздание с сохранением старых реализованных объектов до передачи ввода новым; это предотвращает позднюю потерю PRIMARY-выделения X11. Поддеревья без Edit используют переподчинение. Win32 сохраняет HWND при прямом переносе. Логические уведомления и атомарность одинаковы; подробности в [GTK](dynamic-tree-gtk4-integration.md), [Win32](dynamic-tree-win32-integration.md) и [общей интеграции](dynamic-tree-core-native.md).

| Проверка координатора | Результат |
|---|---|
| Полный `CGO_ENABLED=0 go test -count=1 ./...`, Linux build/vet | PASS |
| Windows amd64 CGO_ENABLED=0 build/vet, компиляция двух test EXE | PASS сборки; не native acceptance |
| Все 8 GTK-тестов с `GOLPHI_GTK_NATIVE=1` | PASS, включая физическую финализацию и clipping 1200/0 |
| Активное GTK-окно с `GTK_CSD=1`, Tree/ReplacementSelection | PASS, смещение root `[0 46 640 434]` сохранено до и после frame |
| Старые native-core/native-close и новый dynamic core→native→core | PASS |
| `CGO_ENABLED=1 go test -race ./ui ./internal/backend` | PASS |
| Реальный Win32, clipping и foreground другого приложения | Ожидаются Windows-логи и ручные наблюдения |

Команды выполнялись с `GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off`. Нативные Linux-команды: `DISPLAY=:0 GDK_BACKEND=x11 GOLPHI_GTK_NATIVE=1 CGO_ENABLED=0 go test ./internal/backend/gtk4 -count=1 -v -timeout 30s`; затем `DISPLAY=:0 GDK_BACKEND=x11 GOLPHI_NATIVE_TREE=1 GOLPHI_NATIVE_CORE=1 GOLPHI_NATIVE_WM_CLOSE=1 CGO_ENABLED=0 go test ./ui -run '^TestNative(CoreIntegration|DynamicTreeIntegration)$' -count=1 -v -timeout 45s`. CSD-регрессия: первая команда с `GTK_CSD=1 -run '^TestNative(Tree|ReplacementSelection)$'`. Окружение прежнее: Linux Mint 22.3 amd64, Go 1.26.5, GTK 4.14.5, X11; переменные GDK_SCALE/GDK_DPI_SCALE не заданы.

Для Windows подготовлен `artifacts/golphi-dynamic-native-windows.zip`: два консольных EXE, русская инструкция, runner и SHA256. Команды сборки и хеши — [manifest](../artifacts/dynamic-tree-native-build.txt). PowerShell-скрипт запускает автоматические тесты, а `-Manual` добавляет ограниченный по времени интерактивный сценарий. Подтверждения оператора и машинные наблюдения сохраняются раздельно; manual PASS не выдаётся за автоматическое сравнение пикселей. PowerShell и Windows EXE здесь не выполнялись.

Fake fault injection, API-boundary fault injection, старые токены и реальные native-сигналы различены в отчётах. Не добавлены обещания для IME, Wayland, DPI switching, нескольких форм, cleanup retry и остальных открытых вопросов. До фактической Windows-проверки задачи 4.2–4.4 и общий этап приёмки остаются открытыми.

Независимое review GTK после исправлений — PASS; review Win32 одобряет готовность к нативному запуску. Общий integration checkpoint — PASS. Подсказка runner дополнена путём инструкции внутри архива. Strict validation изменения и пяти основных specs, `git diff --check` — PASS. Завершена задача 4.1; Windows-зависимые пункты не отмечены выполненными.

### Полученный отказ Windows — 2026-10-29

Пользователь запустил автоматическую проверку на Windows build 26100. Хеши backend/UI совпали с manifest переданного пакета. `TestNativeDynamicTreeWin32` завершился FAIL на `tree_native_windows_test.go:120`: `selection direction cannot be established unambiguously`, `EXIT_CODE=1`. Это сценарий сфокусированного обратного выделения перед прямым переносом. [Журнал](../artifacts/dynamic-tree-windows-failure-20261029/backend-native.log.txt) сохранён с декодированием UTF-16 в UTF-8; [окружение](../artifacts/dynamic-tree-windows-failure-20261029/environment.txt) — с нормализацией UTF-8 BOM.

Исключение PowerShell на скриншоте сообщает о ненулевом коде теста; это не ошибка разбора `.ps1`. Отдельно исправлена кодировка русского runner: UTF-8 с BOM, правило закреплено в AGENTS.md. Windows-приёмка остаётся открытой; исходный краткий журнал не различает фазу снимка и не содержит native-координат, поэтому сам по себе не устанавливает причину неоднозначности.

Подготовлен отдельный `artifacts/golphi-dynamic-native-windows-diagnostic.zip` с фазами отказа и read-only native-трассировкой. [Новый manifest](../artifacts/dynamic-tree-native-diagnostic-build.txt). Это диагностическая сборка, не подтверждённое исправление выделения. Полный CGO_ENABLED=0 набор тестов и Linux/Windows vet прошли; Windows test EXE пересобраны. Требуется повторный автоматический запуск, ручная проверка пока не нужна.
