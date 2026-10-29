# Core → native → core: динамическое дерево

`ui/tree_native_test.go`, `TestNativeDynamicTreeIntegration` — opt-in проверка
производственного ядра с настоящим платформенным `Driver`. Запуск:

```sh
GOLPHI_NATIVE_TREE=1 CGO_ENABLED=0 go test ./ui -run '^TestNativeDynamicTreeIntegration$' -v -count=1
```

Нужен доступ к графической сессии. Linux использует GTK/X11; Windows требует
реального Windows desktop. Каждый сценарий запускается отдельным процессом на
startup UI thread; внешний timeout 25 секунд завершает зависший процесс.
Cross-compilation подтверждает только сборку.

Сценарий материализует вложенные Panel/Edit во время Run, проверяет отсутствие
OnChange при материализации, создаёт pending у двух потомков и переносит их
общего предка между присоединёнными Panel. Сохраняются pending, база и поколение
уведомления; после dispatch каждый Edit получает ровно одно повторное OnChange.
Фактический текст затем сверяется с Driver.Text.

Driver.SetText на живом Edit вызывает настоящий native Change. В его обработчике
сценарий отсоединяет и повторно присоединяет предка: накопленное уведомление
отменяется, тихий Unicode-текст становится новой базой, возврат к входному A
создаёт ровно одно отложенное уведомление. Native сигналы технических операций
не должны добавлять прикладных вызовов.

Поздние события старого peer/поколения **внедряются** через сохранённый callback
ядра до и после reattach. Это проверка core routing, а не искусственный вызов
освобождённого native handle и не доказательство позднего OS callback. Безопасность
native callback roots отдельно проверяют тесты адаптеров.

Второй настоящий native Change уничтожает пересекающиеся поддеревья Owner и
Parent. Логические объекты сразу уничтожены; Driver.Destroy ещё не вызван до
выхода callback. Следующий внешний dispatch проверяет ровно один core-вызов
Destroy для каждого опубликованного peer. После Run таблицы ядра, native peer
registry, deferred retirements и callback closures должны быть пусты.
Внутренние физические освобождения отсоединённых peer не проходят через
обёртку Driver.Destroy: их однократность требует отдельных счётчиков тестов
адаптера, её этот observer не утверждает.

| Проверка | Linux GTK | Windows Win32 |
|---|---|---|
| Сборка тестов | PASS, вместе с нативным тестом | cross-build; native запуск ожидается |
| Реальный dynamic core→native→core | PASS, 2026-10-29 | ожидается запуск пользователем |
| Старое поколение через callback ядра | внедрение в реальном сценарии | внедрение в реальном сценарии |
| Late callback физического native root | ответственность тестов адаптера | ответственность тестов адаптера |

Таблица отделяет интеграцию от fault injection; отсутствие display и пропущенный
opt-in тест не означают подтверждения native поведения. Проверки fault injection
обычного/фатального отката общего ядра описаны в dynamic-tree-runtime.md и
не подменяют реальные сбои native API.

Фактический Linux-прогон 2026-10-29: `DISPLAY=:0 GDK_BACKEND=x11 GOLPHI_NATIVE_TREE=1 CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go test ./ui -run '^TestNativeDynamicTreeIntegration$' -v -count=1` — PASS. Вывод: `OnChange=4/2 nativeChanges=12 coreReleased=7; old-epoch events=injected`. Окружение: Linux Mint 22.3 amd64, GTK 4.14.5, X11, Go 1.26.5; переменные масштаба не заданы.
