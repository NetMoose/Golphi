# Область реализации динамического дерева

Этот документ описывает реализацию активного изменения
[implement-dynamic-component-tree](../openspec/changes/implement-dynamic-component-tree/proposal.md).
Действующие основные OpenSpec остаются ранее принятым базовым контрактом
[lifecycle](../openspec/specs/component-lifecycle/spec.md),
[events](../openspec/specs/component-events/spec.md),
[failure](../openspec/specs/application-failure/spec.md),
[runtime](../openspec/specs/native-ui-runtime/spec.md) и
[Post](../openspec/specs/post-dispatch/spec.md). Перенос дельт в основные Scope
и Requirements выполняется отдельной синхронизацией.

- Экспериментальный API меняет сигнатуры несовместимо. Owner имеет роль
  `Component`, Parent — `Container`. У обычного компонента Owner обязателен
  и фиксируется при создании. Form и минимальная вложенная Panel выполняют обе
  роли. В публичном API этого этапа приняты роли и Panel. Сторонняя реализация
  компонентов, публичные нативные типы и публичный API ввода за пределами этапа.
- Во время единственного Run динамическое создание, SetParent и SetBounds
  выполняются на UI-потоке. Worker передаёт работу через Post. Логическое
  поддерево материализуется целиком до успешного возврата SetParent при
  присоединении к работающей форме. Отрисовка может состояться позже.
  Panel обрезает детей по своей внутренней области. SetBounds доступен
  до Run и для живых неприсоединённых контролов во время Run.
  Фиксированная клиентская область и начальный масштаб сохраняются.
- При прямом переносе сохраняются текст, прикладное состояние OnChange
  и фокус. Внешний фокус остаётся снаружи. Каретка, диапазон и активный конец
  выделения сохраняются у того же native Edit при переносе и обычном откате.
  После пересоздания Edit позиции не определены. Это относится и к GTK-переносу
  с заменой peer, и к detach/reattach. Reattach сохраняет фактический или
  тихо подготовленный текст, устанавливает новую базу OnChange и не захватывает
  фокус. Получатель фокуса сразу после detach определяется нативной системой.
- Обычный сбой сохраняет работоспособное прежнее состояние и возвращает
  диагностику операции и очистки. При невозможности восстановления закрываются
  прикладная доставка и приём Post, ещё не начатая работа отменяется.
  Итоговая диагностика Run собирается с учётом очистки после выхода из callback.

## Требования и проверки

Названия в таблицах совпадают с заголовками дельт. «Ядро» обозначает проверки
логики, в том числе сценарии Run с fake Driver. «Инъекция» обозначает заданный
тестом сбой. Нативные тесты используют настоящий адаптер и графическую сессию.
Даты и степень подтверждения приведены в разделе [Свидетельства](#свидетельства).

### [Component lifecycle](../openspec/changes/implement-dynamic-component-tree/specs/component-lifecycle/spec.md)

| Требование | Покрытие |
| --- | --- |
| Transactional parent assignment | Ядро и инъекция. `TestTreeAttachmentTransitions` проверяет сохранение старого размещения и состояния двух потомков при позднем сбое, `TestTreeMaterializationRollbackRetry` — сохранение подготовленной структуры, настроек и дальнейшую доставку после повторной попытки. Нативная очистка частично созданных ресурсов проверяется на обеих ОС. [Ядро](../ui/tree_run_test.go), [GTK](#gtk), [Win32](#win32). |
| Fixed pre-run geometry for MVP | `TestGeometryPreservesLogicalCoordinates`, `TestGeometryRejectsInvalidValuesWithoutMutation`, `TestGeometryFrozenWhenRunStarts` проверяют координаты, ошибки размеров и ограничения setters. Точную область 400×180, координаты и минимум темы измеряет GTK `TestNative`. Проверка фиксированной геометрии также относится к принятому MVP. [Ядро](../ui/geometry_test.go), [GTK](../internal/backend/gtk4/native_test.go), [MVP](mvp-validation.md). |
| Fixed window size and startup scale for MVP | GTK `TestNative` читает флаг resizable. Win32 `TestStartupScale` проверяет пересчёт начального масштаба. Свежего отдельного прогона действий пользователя по изменению размера Windows для этой дельты нет. [GTK](../internal/backend/gtk4/native_test.go), [Win32](../internal/backend/win32/text_test.go), [MVP](mvp-validation.md). |
| General ownership and container roles | `TestGeneralComponentRoles` и `TestGeneralOwnerValidation` проверяют разные Owner и Parent, каскад, пустого, typed-nil и уничтоженного владельца. [Ядро](../ui/roles_test.go). |
| Minimal nested panel | `TestOverlappingCascadeDuringChildCallback` проверяет немедленное логическое уничтожение и однократный release после callback. Сценарий core/native проверяет количество вызовов Driver.Destroy ядром. GTK `TestNativePanelClipping` проверяет границы и геометрию, Win32 `TestNativeDynamicTreeManual` и шаг 2 примера — видимую обрезку. [Ядро](../ui/cascade_run_test.go), [Core/native](#core-native), [GTK](#gtk), [Win32](#win32), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Dynamic logical construction and subtree attachment | `TestLogicalPreparationRunningThreadGuards` проверяет тихую логическую подготовку, `TestTreeMaterializationRollbackRetry` — готовность узлов и свойств на возврате, `TestNestedRolesStartup` — предварительно собранную вложенность при Run. В примере шаг 1 проверяет создание из OnClick и Post. [Подготовка](../ui/preparation_run_test.go), [транзакция](../ui/tree_run_test.go), [startup](../ui/roles_run_test.go), [core/native](#core-native), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Atomic subtree parent transaction | `TestTreeMaterializationRollbackRetry` вводит поздний сбой ребёнка и проверяет отсутствие частичной публикации, сохранение связей и успешный retry. В нативных адаптерах проверены поздний отказ узла и отсутствие частичного результата. [Ядро](../ui/tree_run_test.go), [GTK](#gtk), [Win32](#win32). |
| Parent validation precedes native changes | `TestParentValidationOwnerRootAndCycles`, `TestParentValidationRunningPreservesEvents`, `TestParentValidationDestroyedReceiverFirst` проверяют self-parent, циклы, Owner-корень, уничтоженную цель, приоритет уничтоженного получателя и сохранение событий. [Ядро](../ui/parent_validation_test.go). |
| Same parent assignment is inert | `TestSameParentInertDuringRun` проверяет повтор текущего Parent и nil, отсутствие изменений native-состояния и событий. Применение потоковых guards к no-op проверяет `TestLogicalPreparationRunningThreadGuards`. [No-op](../ui/cascade_run_test.go), [guards](../ui/preparation_run_test.go). |
| Detached geometry while running | `TestLogicalPreparationRunningThreadGuards` проверяет изменение Bounds у потомка отсоединённой панели, включая флаг прежнего запуска. `TestTreeMaterializationRollbackRetry` сверяет применение новых Bounds при присоединении. Шаг 6 примера проверяет тихое изменение ширины после фактического detach. [Guards](../ui/preparation_run_test.go), [транзакция](../ui/tree_run_test.go), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Direct move preserves input state | `TestTreeInputState` проверяет текст, внутренний и внешний фокус, восстановление ввода при позднем сбое. GTK `TestNativeReplacementTextFocus` проверяет замену Edit с сохранением текста и правилами фокуса. Win32 `TestNativeRetainedHWNDTransferWin32` проверяет сохранённый HWND и позднюю ошибку. Core/native проверяет сохранение pending/base при прямом переносе. Шаги 3–4 наблюдают ввод после переноса. [Ядро](../ui/tree_input_test.go), [GTK](#gtk), [Win32](#win32), [core/native](#core-native), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Retained Edit preserves positions | `TestTreeInputState` проверяет обратный диапазон с кириллицей и символами вне BMP. `TestSelectionScalarUTF16` проверяет преобразование индексов, `TestShiftEndNativeTransition` — обработку наблюдаемого перехода в переносимом тесте. Win32 проверяет идентичность HWND, диапазон/активный конец при переносе и откате, а отдельная клавиатурная проба — продолжение выделения реальным вводом. Ручной повтор Shift+End отражён ниже. [Ядро](../ui/tree_input_test.go), [индексы и переход](../internal/backend/win32/input_test.go), [Win32](#win32), [ручная приёмка](#свидетельства). |
| Detach preserves text without restoring focus | `TestTreeInputState` проверяет неизменный и подготовленный текст на новом peer, тишину и сохранение внешнего фокуса. Нативные тесты адаптеров проверяют текст и фокус, core/native — тишину; шаги 5–6 примера проверяют поведение на обеих ОС. [Ядро](../ui/tree_input_test.go), [GTK](#gtk), [Win32](#win32), [core/native](#core-native), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Detached text edit resets selection | `TestTreeInputState/changed`, `/changed-back` и `/empty` проверяют логический сброс, `/same` — сохранение снимка. После reattach проверяются текст, тишина и внешний фокус. Позиции нового peer не входят в критерий. [Ядро](../ui/tree_input_test.go). |

### [Component events](../openspec/changes/implement-dynamic-component-tree/specs/component-events/spec.md)

| Требование | Покрытие |
| --- | --- |
| Detachment cancels pending text notifications | `TestTreeAttachmentTransitions`, `TestTreeReattachmentComparison` и core/native проверяют отмену после успешного detach и смену поколения. `TestTreeRecoverableCleanup` проверяет сохранение pending при обычном сбое переноса, Win32 retained-тест — обычный откат после неудачного detach со скрытием peer. Отдельный сквозной сценарий с pending и отказом именно `SetParent(nil)` не зафиксирован. [Переходы](../ui/tree_run_test.go), [инъекция в ядре](../ui/tree_recoverable_test.go), [core/native](#core-native), [Win32](#win32). |
| Attachment delivers no preparation events | `TestTreeMaterializationRollbackRetry` проверяет тишину при подготовке, на сбое и внутри незавершённой операции, затем обычную доставку после успеха. Core/native проверяет тишину успешной материализации и настоящий последующий Change. [Ядро](../ui/tree_run_test.go), [core/native](#core-native). |
| Reattachment starts a fresh comparison period | `TestTreeReattachmentComparison` и core/native проверяют новую базу и отложенный повтор при возврате к тексту входа в callback, тишину без дальнейших изменений и переход через предка. [Ядро](../ui/tree_run_test.go), [core/native](#core-native). |
| Direct attached transfer preserves notification state | `TestTreeAttachmentTransitions` и core/native сохраняют pending, базу и поколение двух потомков, доставляют повтор после переноса и подавляют технические сигналы. Обычная поздняя ошибка переноса покрыта инъекцией в ядре. [Ядро](../ui/tree_run_test.go), [core/native](#core-native). |
| Transfer to detached container counts as detachment | `TestTreeAttachmentTransitions` использует неприсоединённую Panel как ненулевой Parent и проверяет отмену у потомков, тишину и сохранение внутреннего дерева. Сохранение pending при обычном отказе проверено на прямом переносе. Отдельный отказ переноса именно в неприсоединённую Panel с ожидающим OnChange не зафиксирован. [Ядро](../ui/tree_run_test.go). |

### [Application failure](../openspec/changes/implement-dynamic-component-tree/specs/application-failure/spec.md), [native UI runtime](../openspec/changes/implement-dynamic-component-tree/specs/native-ui-runtime/spec.md) и [Post dispatch](../openspec/changes/implement-dynamic-component-tree/specs/post-dispatch/spec.md)

| Требование | Покрытие |
| --- | --- |
| Failed tree operation retains cleanup diagnostics | `TestApplyTreeFailureRetainsDiagnosticsAndContinuesCleanup` и `TestTreeRecoverableCleanup` вводят ошибку и панику со стеком, проверяют продолжение независимой очистки и работоспособной доставки. Нативный Win32 `TestNativeAbortedTreeCleanupWin32` внедряет сбой DestroyWindow в трёх сценариях неудачного присоединения. [Транзакция](../internal/backend/tree_test.go), [ядро](../ui/tree_recoverable_test.go), [диагностика](dynamic-tree-failures.md), [Win32](#win32). |
| Unrecoverable tree operation stops application | `TestTreeFatalRollback/tree-fatal-placement` и `/tree-fatal-first-attachment` с fake Driver проверяют ошибку операции и восстановления, закрытие доставки сразу, возврат из callback до teardown, панику очистки со стеком и итог Run. Спонтанный фатальный сбой оконной системы не наблюдался. [Инъекция](../ui/tree_fatal_test.go), [runtime](dynamic-tree-runtime.md). |
| Dynamic tree operations require the UI thread | `TestLogicalPreparationRunningThreadGuards` проверяет panic в worker до изменения графа и вызовов адаптера, затем выполнение операций через принятый Post. Шаг 1 примера наблюдает обе точки входа в реальной сессии. [Ядро](../ui/preparation_run_test.go), [пример](dynamic-tree-example.md#ручной-сценарий-на-каждой-ос). |
| Unrecoverable tree failure closes dispatch | `TestTreeFatalRollback` проверяет отмену ранее принятых Post, отсутствие новой доставки во время очистки и допустимость результатов гонки worker с границей остановки. [Инъекция](../ui/tree_fatal_test.go), [runtime](dynamic-tree-runtime.md). |

## Свидетельства

### Core-native

`TestNativeDynamicTreeIntegration` проводит настоящий native Change до
прикладного обработчика, затем проверяет дерево, маршруты и ресурсы. События
старого поколения вводятся через сохранённый callback ядра. Счётчик release
в этом тесте относится к вызовам Driver.Destroy из ядра. Физическое освобождение
peer проверяется отдельно тестами адаптеров. [Тест](../ui/tree_native_test.go),
[описание и Linux-прогоны](dynamic-tree-core-native.md),
[Win32, 13 ноября](../artifacts/dynamic-tree-cleanup-windows-20261113/ui-native.log.txt).

### GTK

`TestNativeTree`, `TestNativePanelClipping` и `TestNativeReplacementTextFocus`
проверялись на Linux/X11. В ветках отказа используются заданные тестом условия
и подмена вызовов на API-границе. При успешном переносе с пересозданием проверяются
текст и фокус. [Тесты](../internal/backend/gtk4/tree_native_test.go),
[отчёт адаптера](dynamic-tree-gtk4-integration.md),
[прогоны 4.5 и 4.4](dynamic-tree-validation.md).

### Win32

- Автоматические тесты `TestNativeDynamicTreeWin32`,
  `TestNativeRetainedHWNDTransferWin32` и `TestNativeAbortedTreeCleanupWin32`
  прошли 13 ноября вместе с core/native на Windows 11 amd64 build 26100.
  Пользовательские машинные SHA256 backend/UI совпадают с manifest этой сборки.
  DPI в автоматическом прогоне отдельно не записан. Отказ DestroyWindow и
  поздние отказы транзакции вводятся тестом. [Широкий тест](../internal/backend/win32/tree_native_windows_test.go),
  [retained-тест](../internal/backend/win32/retained_tree_native_windows_test.go),
  [сбой очистки](../internal/backend/win32/tree_cleanup_native_windows_test.go),
  [журнал backend](../artifacts/dynamic-tree-cleanup-windows-20261113/backend-native.log.txt),
  [журнал retained](../artifacts/dynamic-tree-cleanup-windows-20261113/backend-retained.log.txt),
  [журнал очистки](../artifacts/dynamic-tree-cleanup-windows-20261113/backend-aborted-cleanup.log.txt),
  [окружение](../artifacts/dynamic-tree-cleanup-windows-20261113/environment.txt),
  [manifest](../artifacts/dynamic-tree-cleanup-review-build.txt).
- `TestNativeDynamicTreeManual` от 2 ноября при 96 DPI зафиксировал внешний
  фокус и clipping с подтверждением оператора. Пиксельного сравнения не проводилось.
  `TestNativeEditKeyboardSelectionProbe` даёт отдельное нативное свидетельство
  для обратного выделения и его продолжения. [Ручной тест](../internal/backend/win32/manual_native_windows_test.go),
  [журнал оператора](../artifacts/dynamic-tree-windows-20261102/manual-operator.log.txt),
  [снимок](../artifacts/dynamic-tree-windows-20261102/clipping.png),
  [клавиатурная проба](../internal/backend/win32/selection_keyboard_probe_windows_test.go),
  [журнал клавиатурной транзакции](../artifacts/retained-hwnd-windows-20261102/keyboard-transaction.log.txt).

### Ручной пример и сборки

- **Linux/X11, 13 ноября.** Пользователь сообщил «почти корректно» и отметил
  выделение всей строки при переносе. GTK пересоздаёт Edit. Позиции нового peer
  вне критериев, поэтому отзыв принят для согласованного объёма. Отдельного
  нового прогона Linux 20 ноября и нового отчёта с измерениями по каждому шагу
  нет. [Отзыв](../artifacts/dynamic-example-feedback-20261113/observations.md),
  [приёмка примера 5.1](dynamic-tree-example.md#результаты-приёмки-51).
- **Windows, 20 ноября.** После передачи кандидата `e1e2be3` пользователь
  подтвердил Shift+End и PASS остальных сценариев. Shift+Home подтверждён
  в предыдущем повторе. SHA256
  `ba6e966f8413e8b5b013870c735fb3d3df13c327554e959d6566993548b6d180`
  относится к подготовленному EXE. Новый машинный журнал с хешем запущенного
  файла, DPI и окружением для этого ручного повтора не получен.
  [Приёмка примера 5.1](dynamic-tree-example.md#итоговое-подтверждение--2026-11-20),
  [manifest Shift+End](../artifacts/dynamic-example-shift-end-build.txt).
- **Сборки 5.2, 20 ноября.** `hello` и `dynamic-tree` собраны для Linux и
  Windows amd64 с `CGO_ENABLED=0` и недоступными CC/CXX. Проверены GUI-подсистема
  Windows-примеров и console-подсистема тестовых EXE. Хеши фиксируют именно
  эти файлы, новых запусков примеров и нативных тестов в этой проверке нет.
  [Команды и форматы](dynamic-tree-builds.md),
  [SHA256](../artifacts/dynamic-tree-builds-20261120.sha256).

## Открытые вопросы

Сохранение позиций и направления выделения после пересоздания Edit перенесено
на следующий этап. В силе весь перечень [Open Questions](../openspec/changes/implement-dynamic-component-tree/design.md#open-questions),
включая несколько окон и межформенные переносы, Owner, layout и изменение DPI,
публичные Application и API ввода, правила Post за пределами принятого
контракта, диагностику, зависимую очистку, retry и сочетания после фатального
отказа. Для произвольных клавиатурных сочетаний, bidi/IME и внешнего вида Win32
новых критериев приёмки нет. Оформление Win32 ведётся как WIN-01. [Backlog](backlog.md).
