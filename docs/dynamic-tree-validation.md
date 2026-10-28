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
| Windows: фактический запуск нового native-примера | Ожидается пользовательский журнал |
| Общий Driver для динамического дерева | Ожидает результатов двух нативных экспериментов |

Технические пробы не являются реализацией публичного динамического API. Ранее принятые MVP-журналы не доказывают новых свойств дерева. Cross-build отдельно от Windows native-прогона. Остальные открытые вопросы сохранены; основные specs не синхронизируются до отдельного запроса.

## Свидетельства первого checkpoint

2026-10-28: Linux Mint 22.3 amd64, Go 1.26.5, пакет GTK4 `4.14.5+ds-0ubuntu0.10`, X11 `DISPLAY=:0`, `GDK_BACKEND=x11`. Переменные GDK_SCALE/GDK_DPI_SCALE не заданы; отдельная матрица масштабов не проверялась. Журнал получен командой:

```sh
GOLPHI_TREE_PROBE=1 CGO_ENABLED=0 GDK_BACKEND=x11 DISPLAY=:0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go test -count=1 -v ./cmd/probe-tree-linux
```

Нативный эксперимент подтверждает вложенность без роста от ребёнка 1200×600, clipping через hit-test и отрисовку GTK (1200 изменённых пикселей внутри, 0 снаружи), перенос и пересоздание Edit с сохранением фокуса/Unicode-позиций, отсутствие auto-scroll при фокусе, disconnect и финализацию двух Edit. Это не полная интеграция динамического API. Экспериментальная пауза для смены фокуса не принимается как production-протокол: следующий этап должен найти событийное решение с сохранением синхронного контракта SetParent либо сообщить blocker.

Windows-проба проверена ревью, собрана с `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`; её native-проверки, DPI и PowerShell runner ещё требуют запуска на Windows. До фактического журнала задача 1.3 и зависимая задача 1.4 остаются открытыми. Полный технический этап 1.5 пока не завершён; текущий commit/push сохраняет подготовленный checkpoint.

Финальные проверки checkpoint: `CGO_ENABLED=0 go test -count=1 ./...`, `go vet ./...` с теми же offline caches; Windows cross-build и target vet для `cmd/probe-tree-windows`; strict validation изменения и всех пяти основных specs. Общий go test без opt-in не запускает native-тесты.
