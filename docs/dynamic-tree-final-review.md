# Итоговое review ветки — задача 5.4

Дата checkpoint: 20 ноября 2026 года. Проверена вся ветка от `82605a8` до
`f2c29f0`, затем исправления замечаний этого review. Нативная приёмка новой
Windows-сборки пока ожидается, поэтому 5.4 остаётся открытой.

## Review и исправления

1. **Порядок физического освобождения.** Перенос в позже созданную Panel мог
   оставить старый порядок release: backend каскадно удалял ребёнка раньше
   повторного Destroy ядром. Реестр теперь учитывает актуальный порядок native
   родителей. Для уже логически уничтоженных детей сохраняется native parent
   до физического release. Если родитель сохраняется, дети освобождаются первыми;
   если он отсоединён/пересоздан, старое поддерево передано backend и ядро удаляет
   его устаревшие bindings. Это учитывает Destroy ребёнка и перенос/отсоединение
   его панели в одном callback, без опасного release активного callback.
2. **Гонка guardAlive.** Уничтожение фиксируется атомарно. Приоритет уничтоженного
   получателя перед UI-thread guard сохранён. Остальные поля и API не получают
   новых обещаний потокобезопасности.
3. **Удержание уничтоженных компонентов.** После безопасного успешного release
   уничтоженные участники, включая никогда не материализованных, удаляются из
   runtime-списков; освободившиеся элементы обнуляются. Ошибки cleanup сохраняют
   прежнюю диагностику и финальный Close. Политика retry не расширяется.

Все три замечания устранены по повторному review исходного кода. До исправлений
новые тесты воспроизводили повторный release, удержание объектов и гонку.
После исправлений регрессии прошли. Для вложенного случая с ожидающим release
потребовалось дополнение первоначального исправления; отдельный повторный
GTK-сценарий принудительно пересоздаёт панель с живым Edit-соседом.

## Сценарии MODIFIED и guards

Матрица [Scope](dynamic-tree-scope.md) сверена с пятью дельтами: 23 требования,
приоритеты уничтоженного получателя и UI-thread guards, no-op, обычная ошибка
и аварийный откат не подменены новыми правилами. Два пробела покрытия 5.3 закрыты
[TestTreeFailedDetachmentPreservesPendingChange и TestTreeFailedMoveToDetachedPanelPreservesPendingChange](../ui/tree_pending_failure_test.go):
поздний отказ после частичного native-действия, прежние bindings/Parent,
pending/base/generation, отсутствие технических событий и однократная доставка.
Тесты чувствительны к преждевременной отмене и пропуску rollback (проверено overlays).

[Регрессии жизненного цикла](../ui/tree_lifetime_test.go):
`TestMovedPeerRelease`, `TestPendingChildRelease`, `TestTransientLifetime`,
`TestDestroyWorkerGuard`. [Core/native](../ui/tree_native_test.go) теперь включает
`native-tree`, `native-pending-child-move`, `native-pending-child-detach`.
Fake с каскадным удалением моделирует backend; заданные ошибки остаются fault
injection, а не свидетельством спонтанного системного сбоя.

## Проверки

Окружение: Go 1.26.5 linux/amd64, Linux Mint 22.3, GTK 4.14.5,
X11 DISPLAY=:0, GDK_BACKEND=x11. Команды используют GOCACHE=/tmp/golphi-gocache,
GOPROXY=off и локальный module cache.

| Команда / проверка | Результат |
|---|---|
| `CGO_ENABLED=0 go test -count=1 ./...` | PASS после окончательного исправления |
| `CGO_ENABLED=0 go vet ./...` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...` | PASS |
| `CGO_ENABLED=1 go test -race -count=1 ./ui ./internal/backend` | PASS, UI 57.783s, backend 1.034s |
| `GOLPHI_GTK_NATIVE=1 CGO_ENABLED=0 go test -count=1 -v ./internal/backend/gtk4` | PASS, 8 тестов, 2.023s; backend не менялся при исправлении ядра |
| `GOLPHI_NATIVE_TREE=1 CGO_ENABLED=0 go test -count=1 -v ./ui -run '^TestNativeDynamicTreeIntegration$'` | PASS после окончательного исправления, 3 подслучая, 0.348s; OnChange=4/2, nativeChanges=12, coreReleased=8 |
| Windows `go test -c -buildvcs=false` для backend и UI, CGO_ENABLED=0 | PASS, консольные EXE |

Race-проверка использует CGO_ENABLED=1 только для race runtime; сборки приложений
и остальные проверки без CGO сохранены. Предыдущие Windows-логи и ручное
подтверждение Shift+End относятся к своим сборкам. Они не заменяют запуск
изменённого core/native теста после новых исправлений.

## Оставшийся Windows-прогон

Пакет: `artifacts/golphi-dynamic-tree-final-review-windows.zip`.
[Команды и SHA256 сборки](../artifacts/dynamic-tree-final-review-build.txt).
Из новой папки выполнить:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-dynamic-tree-native-windows.ps1 -BackendExe .\golphi-win32.test.exe -UIExe .\golphi-ui.test.exe -LogDirectory artifacts/final-review-windows
```

Четыре автоматические проверки без SKIP: cleanup, широкий backend, retained HWND,
core/native с тремя подслучаями. Передать всю папку `artifacts/final-review-windows`.
Не добавлять -Manual: прежние ручные сценарии повторять не требуется. Скрипт и
README в UTF-8 с BOM; содержимое ZIP и хеши проверены. Новых Windows-результатов
на момент checkpoint нет. 5.4 будет отмечена после фактического прогона.

Основные specs, активный design/Open Questions и исторические архивы не
синхронизировались. Новых решений по остальным открытым вопросам нет.
PR остаётся отдельной задачей 5.5.
