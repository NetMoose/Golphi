# Spec Delta

## ADDED Requirements

### Requirement: Immediate logical destruction
Destroy() SHALL немедленно делать компонент логически уничтоженным. Destroyed() SHALL безопасно возвращать bool и показывать true после логического уничтожения. Повторный Destroy() MUST быть безопасным и ничего не менять, включая период ожидания физического освобождения.

#### Scenario: Destroy twice before physical release
- **WHEN** компонент уничтожают, физическое освобождение ещё отложено, затем вызывают Destroyed() и Destroy() снова
- **THEN** Destroyed() возвращает true, повторный Destroy() ничего не меняет и не инициирует повторное освобождение

### Requirement: Invalid operations on destroyed components
Обычные операции над логически уничтоженным компонентом SHALL вызывать panic. Исключения — безопасные Destroy() и Destroyed(). Возврат error методом не заменяет panic при использовании уничтоженного объекта.

#### Scenario: Access after destruction
- **WHEN** после уничтожения вызывают обычный getter, setter или SetParent
- **THEN** каждый такой вызов вызывает panic, а SetParent не подменяет её обычным error

### Requirement: Deferred physical release during callbacks
Физическое освобождение ресурсов SHALL допускать отсрочку до выхода из callback, когда это необходимо. Отсрочка MUST NOT откладывать логическое уничтожение. Уже выполняющийся обработчик SHALL продолжать работу после Destroy(), если сам не вызывает panic.

#### Scenario: Self destruction during a handler
- **WHEN** обработчик уничтожает свой компонент и продолжает работу без обычных обращений к нему
- **THEN** Destroyed() уже возвращает true, обработчик продолжает выполнение, а физическое освобождение при необходимости происходит после выхода из callback

#### Scenario: Access from the remaining handler body
- **WHEN** после Destroy() текущий обработчик вызывает обычную операцию уничтоженного компонента
- **THEN** операция вызывает panic, обрабатываемую контрактом application-failure

### Requirement: Immediate semantics apply to the destruction cascade
Каждый компонент, уничтожаемый существующим каскадом по Owner и Parent, SHALL подчиняться правилам логического уничтожения и безопасного повторного Destroy(). Эти правила MUST сохранять отсутствие повторного освобождения при пересечении деревьев.

#### Scenario: Destroying the panel from a child handler
- **WHEN** обработчик кнопки уничтожает её панель, а кнопка уничтожается каскадно
- **THEN** кнопка логически уничтожена; обычное обращение к ней вызывает panic, Destroyed() и повторный Destroy() остаются безопасными
