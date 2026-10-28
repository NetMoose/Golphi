# Spec Delta

## MODIFIED Requirements

### Requirement: Post reports acceptance rather than completion
Post(fn) error SHALL возвращать nil при принятии функции в очередь и ошибку при отклонении. Отклонённая задача MUST NOT попадать в очередь или выполняться. Принятие MUST NOT означать завершение или гарантировать выполнение при последующей аварийной остановке либо обычном закрытии MVP.

#### Scenario: Accepted task
- **WHEN** работающий runtime принимает допустимую задачу Post
- **THEN** вызов возвращает nil как подтверждение принятия, а не завершения

#### Scenario: Rejected task
- **WHEN** Post отклоняет задачу
- **THEN** возвращается ошибка, задача не поставлена в очередь и не будет выполнена

#### Scenario: Acceptance followed by failure
- **WHEN** Post вернул nil, но аварийная остановка началась до выполнения задачи
- **THEN** задача отменяется; ранее возвращённый nil не является обещанием её выполнения

#### Scenario: Acceptance followed by normal closure
- **WHEN** Post вернул nil, но обычное закрытие MVP началось до выполнения задачи
- **THEN** задача отменяется; принятие не обещает её выполнения

### Requirement: Post accepts tasks only while running
Задачи Post SHALL приниматься только во время работы приложения. До запуска Form.Run() и во время подготовки MVP Post SHALL отклоняться с ошибкой. Для MVP приём SHALL открываться после успешной подготовки формы до прикладных событий, без ожидания первой отрисовки. После начала аварийной остановки или обычного закрытия MVP приём MUST быть закрыт.

#### Scenario: Before Run
- **WHEN** Post вызывают до запуска Form.Run()
- **THEN** возвращается ошибка, задача не накапливается для будущего запуска

#### Scenario: After failure shutdown begins
- **WHEN** Post вызывают после начала аварийной остановки
- **THEN** возвращается ошибка без постановки задачи в очередь

#### Scenario: Preparation is not readiness
- **WHEN** Run выполняет native-инициализацию, создание компонентов или начальное размещение
- **THEN** Post возвращает ошибку без накопления задачи

#### Scenario: Ready before first paint
- **WHEN** backend, native-компоненты, свойства, начальное размещение, очередь и пробуждение готовы, но первой отрисовки ещё нет
- **THEN** единый переход в Running открывает приём Post до начала обычной прикладной доставки

#### Scenario: Concurrent Post at readiness boundary
- **WHEN** Post конкурирует с переходом в Running
- **THEN** задача либо отклонена до готовности без очереди, либо принята после готовности; частично подготовленный UI ей недоступен

#### Scenario: Startup fails before readiness
- **WHEN** запуск завершился ошибкой до перехода в Running
- **THEN** приём Post не открывается, отклонённые задачи не выполняются

## ADDED Requirements

### Requirement: Package Post for MVP
MVP SHALL предоставлять ui.Post(func()) error без публичного Application. API SHALL сохранять прежние гарантии goroutine safety, UI-выполнения, FIFO, отсутствия рекурсии, отказа nil и отсутствия владельца. Это экспериментальная поверхность MVP; наличие публичного Application и окончательный получатель Post полной библиотеки не определяются.

#### Scenario: Worker posts through package API
- **WHEN** после готовности фоновая goroutine вызывает ui.Post с допустимой функцией
- **THEN** функция принимается и выполняется по существующему контракту Post, если не отменена

### Requirement: Normal MVP shutdown cancels queued tasks
Начало обычной остановки MVP SHALL закрывать приём Post и отменять все ещё не начатые задачи без восстановления. Новые вызовы SHALL возвращать ошибку без очереди. Уже выполняющаяся функция MUST NOT прерываться принудительно. Гонка с остановкой SHALL иметь однозначный исход: принятие до границы с возможной отменой либо отказ без постановки.

#### Scenario: Normal shutdown wins acceptance race
- **WHEN** обычная остановка началась до принятия конкурентного Post
- **THEN** Post возвращает ошибку, задача не попадает в очередь

#### Scenario: Post accepted before normal shutdown
- **WHEN** задача принята до начала обычной остановки, но ещё не выполняется
- **THEN** Post возвращает nil, остановка отменяет задачу, очистка её не восстанавливает
