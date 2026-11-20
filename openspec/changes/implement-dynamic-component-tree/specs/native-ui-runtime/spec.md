# Spec Delta

## ADDED Requirements

### Requirement: Dynamic tree operations require the UI thread
Во время работы конструкторы обычных компонентов, SetParent и SetBounds SHALL выполняться только на UI-потоке, включая неприсоединённые компоненты. Вызов из другой goroutine SHALL вызывать panic до изменения состояния и native-вызовов. ui.Post остаётся безопасным для любой goroutine; panic ошибочного фонового вызова MUST NOT автоматически перехватываться через Run.

#### Scenario: Worker directly creates or changes tree
- **WHEN** фоновая goroutine во время Run напрямую вызывает конструктор контрола, SetParent либо SetBounds
- **THEN** каждый вызов паникует до изменений и нативных действий; для безопасной проверки panic можно перехватить в самой goroutine

#### Scenario: Worker posts tree update
- **WHEN** worker отправляет функцию через ui.Post и она доходит до выполнения
- **THEN** динамические операции выполняются на UI-потоке и разрешены в пределах остальных правил
