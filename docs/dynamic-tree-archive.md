# Завершение OpenSpec-цикла динамического дерева

20 ноября 2026 года. Реализация влита в develop через
[PR #11](https://github.com/NetMoose/Golphi/pull/11), merge commit
`20a93cd0d3379729d90cde1713f6d34f5cceebee`. Для синхронизации и архивирования
создана отдельная ветка `codex/archive-dynamic-component-tree` от этой develop,
без worktree. Код библиотеки, примеров и тестов не менялся.

## Синхронизация

Все 27 задач и артефакты были завершены до архивирования. Изменение перенесено в
[архив](../openspec/changes/archive/2026-11-20-implement-dynamic-component-tree/proposal.md).
Все 9 файлов, включая `.openspec.yaml` и Open Questions дизайна, сохранены
побайтно; старые архивы не изменены.

В основные specs перенесены 23 требования дельт: 19 добавленных и 4 изменённых.

| Спецификация | Добавлено | Изменено |
|---|---:|---:|
| [application-failure](../openspec/specs/application-failure/spec.md) | 2 | 0 |
| [component-events](../openspec/specs/component-events/spec.md) | 4 | 1 |
| [component-lifecycle](../openspec/specs/component-lifecycle/spec.md) | 11 | 3 |
| [native-ui-runtime](../openspec/specs/native-ui-runtime/spec.md) | 1 | 0 |
| [post-dispatch](../openspec/specs/post-dispatch/spec.md) | 1 | 0 |

Scope согласован с принятым срезом. Purpose и исходные заголовки установленных
specs сохранены полностью. Незатронутые требования и сценарии сохранены;
в изменённых блоках применены описания и сценарии дельт. Повторное сравнение
всех пяти дельт подтвердило отсутствие оставшихся изменений для синхронизации.
Основные specs содержат единственный Requirements и не содержат delta-заголовков.

README и живые ссылки документации ведут к основным specs и новому архиву.
[Реализация и свидетельства](dynamic-tree-scope.md),
[итоговая приёмка](dynamic-tree-final-review.md) сохранены. Исторические статусы
внутри архива и датированных журналов не переписываются как текущие результаты.

## Границы и проверки

Сохранён полный [реестр открытых вопросов](../openspec/changes/archive/2026-11-20-implement-dynamic-component-tree/design.md#open-questions),
включая восстановление позиций пересозданного Edit, несколько окон, смену Owner,
layout/DPI, публичный Application, ввод и прочие события, правила Post вне
принятого контракта, диагностику и зависимую очистку/retry. Q-01–Q-14 общего
дизайна сохранены. DOC-01 и WIN-01 остаются будущими задачами backlog.

Strict validation пяти основных specs, проверка полноты sync и сохранности
архива, 415 локальных ссылок/якорей и git diff --check прошли. Новых runtime/native-прогонов нет: изменяются только документация
и расположение завершённых planning-артефактов.
