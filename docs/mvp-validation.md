# Проверка native UI MVP

Изменение: [implement-native-ui-mvp](../openspec/changes/implement-native-ui-mvp/proposal.md). Выполнение начато; этот отчёт не подтверждает готовность всего MVP или завершение E0/E1.

## Матрица

| Платформа | Конфигурация | Источник |
|---|---|---|
| Linux | Mint 22.3 Zena, amd64, glibc 2.39, GTK 4.14.5, GLib 2.80.0, Cinnamon/X11, DISPLAY=:0 | Локальная проверка окружения |
| Windows | Windows 11 Pro, amd64, build 26100, 96 DPI / 100% | Журнал пользовательского native-прогона 28 октября 2026 года |
| Go | 1.26.5, CGO_ENABLED=0 для проверяемых сборок | go version локально; команды каждого прогона фиксируются ниже |

GTK runtime доступен; GTK development metadata (gtk4.pc) не установлен. Текущая системная настройка CGO_ENABLED=1 не используется для проверочных сборок.

## Технический этап

Подготовка Go-модуля, Linux и Windows FFI/bootstrap probes выполняется до фиксации backend-интерфейса. Probe проверяет узкий путь и не является пользовательским MVP.

Проверка Windows не заменяется успешной кросс-компиляцией. Получен успешный Windows-прогон технической пробы; задача 1.4 выполнена для уточнённой пользователем Windows 11. Последующий этап интерфейса backend учитывает результаты обеих платформ.

## Критерии MVP

| Критерий | Статус |
|---|---|
| M-01: сборки без C toolchain | Не завершён |
| M-02: общий native-пример на двух ОС | Не выполнен |
| M-03: события и текст | Не выполнен |
| M-04: Post и UI-поток | Не выполнен |
| M-05: проверки Run и одна попытка | Не выполнен |
| M-06: обычное закрытие и сбои очистки | Не выполнен |
| M-07: ошибки частичного запуска | Не выполнен |
| M-08: аварийный путь и Destroy | Не выполнен |

Все прежние открытые вопросы из дизайна изменения сохранены. Полные T-01–T-09 не объявляются пройденными; динамическое дерево, отменяемое закрытие, resizing и другие отложенные возможности не реализуются этим техническим этапом.

## Модуль: задача 1.2

Рабочий module path — github.com/NetMoose/Golphi; окончательный публичный path остаётся открытым. Закреплена github.com/ebitengine/purego v0.11.2 (её go.mod требует Go 1.25.0), сборки выполнялись Go 1.26.5.

Проверки прошли:

```sh
GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go mod verify
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go build ./ui github.com/ebitengine/purego
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go build ./ui github.com/ebitengine/purego
```

Это проверка совместимости модулей и двух сборок, а не полный M-01 и не Windows native-прогон. Собственного C bridge нет. Review задачи 1.2: соответствие и качество — PASS.

## Linux: эксперимент 1.3

Импорт ui закрепляет startup thread без загрузки GTK. Subprocess-проверка подтверждает сохранение потока после переключений планировщика и отдельный поток фоновой goroutine. Реальный эксперимент создаёт GtkButton, подключает и синхронно испускает clicked, затем отключает callback и освобождает объект после его возврата.

```sh
CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOLPHI_NATIVE_PROBE=1 DISPLAY=:0 GDK_BACKEND=x11 go test -count=1 -v ./cmd/probe-linux
```

Прогон 28 октября 2026 года с доступом к локальному X11:

```text
GTK=4.14.5 DISPLAY=:0 backend=X11 startup=216438 native=216438 callback=216438 callbacks=1 callback_same_thread=true
PASS
```

Без разрешения доступа к X11 из sandbox gtk_init_check возвращал ошибку. После предоставления доступа тот же тест прошёл. Проверки `CGO_ENABLED=0 go test -count=1 -v ./...` и `go vet ./...` прошли; native-тест без явного GOLPHI_NATIVE_PROBE=1 пропускается. Это не проверка message loop, видимой формы или полного жизненного цикла MVP.

## Windows: эксперимент 1.4

Пользователь 28 октября 2026 года уточнил ОС: Windows 11 вместо ранее ошибочно указанной Windows 10. Матрица MVP исправлена на Windows 11/amd64; совместимость с Windows 10 этим прогоном не подтверждается.

Источник: [исходный журнал пользователя](../artifacts/golphi-windows-probe.txt), сохранён без изменений в UTF-16LE. Программа собрана Go 1.26.5 для windows/amd64 с CGO_ENABLED=0; сборка и переносимые тесты прошли ранее. [Инструкция запуска](windows-probe.md).

Результаты:

- Windows 11 Pro, build 26100, архитектура CPU 9 (AMD64), ОС 64-разрядная.
- Системный масштаб 100%, DPI 96, system-aware.
- Startup/main/native/callback/cleanup: поток 10892; worker: 15524.
- EnumWindows: результат 1, 155 callback, все на потоке 10892, own-window-seen=true.
- DestroyWindow=1; PASS; код завершения 0.

Техническая задача 1.4 выполнена. Это подтверждает создание/освобождение нативного окна, синхронные callbacks и закрепление потока. Полный message loop, Form.Run, Post, прикладные события и критерии M-01–M-08 этим экспериментом не проверены.
