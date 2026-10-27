# Spec Delta

## MODIFIED Requirements

### Requirement: Failure shutdown cancels unstarted tasks
При обнаружении паники прикладного обработчика или функции Post Golphi SHALL отменять все ещё не начатые задачи Post и MUST NOT запускать новые. Новые вызовы Post SHALL отклоняться с ошибкой. Очистка и диагностика, включая сбои отдельных операций очистки после первичной паники, подчиняются application-failure.

#### Scenario: Handler panic with queued tasks
- **WHEN** обработчик события вызывает panic при ожидающих задачах Post
- **THEN** все ещё не начатые задачи отменены и больше не запускаются, новые Post отклоняются

#### Scenario: Task panic with queued tasks
- **WHEN** выполняющаяся функция Post вызывает panic при ожидающих задачах
- **THEN** ожидающие задачи отменены, новые Post отклоняются и применяется остановка по application-failure

