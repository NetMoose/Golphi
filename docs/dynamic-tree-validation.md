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
