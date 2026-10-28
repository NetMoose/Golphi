# component-lifecycle Specification

## Purpose

Определить наблюдаемое поведение владения и визуального размещения компонентов Golphi, включая безопасное отсоединение и уничтожение.

## Requirements

### Requirement: Independent ownership and visual parenting
Golphi SHALL разделять Owner, определяющий владение, и Parent, определяющий визуальное размещение. Изменение Parent MUST сохранять Owner.

#### Scenario: Moving between panels
- **WHEN** кнопка принадлежит форме и успешно переносится с одной панели на другую
- **THEN** Parent становится новой панелью, а Owner остаётся формой

### Requirement: Construction without a visual parent
Golphi SHALL позволять создать визуальный компонент с Owner без Parent, настроить свойства и обработчики, затем присоединить его к контейнеру.

#### Scenario: Preparing a button
- **WHEN** кнопка создана с владельцем, настроена без Parent и затем успешно присоединена
- **THEN** кнопка использует подготовленные свойства и обработчики

### Requirement: Transactional parent assignment
`SetParent(parent)` SHALL возвращать error. При ошибке Golphi MUST сохранять прежнего Parent, Owner, все настройки и обработчики, а также работоспособность прежнего размещения; частично созданные ресурсы MUST освобождаться.

#### Scenario: Failed move of a visible component
- **WHEN** перенос отображаемого компонента в другой контейнер завершается ошибкой
- **THEN** возвращается ошибка, компонент остаётся работоспособным у прежнего Parent со всеми настройками и обработчиками
- **AND** частичные ресурсы неуспешного переноса освобождены

#### Scenario: Failed first attachment
- **WHEN** первое присоединение настроенного компонента без Parent завершается ошибкой
- **THEN** Parent остаётся nil, Owner, настройки и обработчики сохраняются для последующей попытки

### Requirement: Non-destructive detachment
Успешный `SetParent(nil)` SHALL удалять компонент и его визуальное поддерево из отображаемого интерфейса, сохраняя Owner, настройки, обработчики и связи внутри поддерева для повторного присоединения.

#### Scenario: Detach and reattach a panel
- **WHEN** панель с кнопками успешно отсоединяют, затем присоединяют снова
- **THEN** кнопки остаются детьми панели и вновь отображаются вместе с ней с сохранёнными настройками и обработчиками
- **AND** пока панель отсоединена, её потомки также считаются отсоединёнными для правил событий, несмотря на сохранённые внутренние связи Parent (см. component-events)

### Requirement: Cascading destruction
Уничтожение компонента SHALL уничтожать принадлежащие ему компоненты и его визуальных потомков независимо от их Owner. Уничтожаемые компоненты MUST исключаться из обеих связей без повторного освобождения ресурсов при пересечении деревьев.

#### Scenario: Destroying a panel with form-owned children
- **WHEN** уничтожается панель, содержащая кнопку, Owner которой является форма
- **THEN** кнопка уничтожается и исключается из списка компонентов формы
- **AND** последующее уничтожение формы не освобождает ресурсы кнопки повторно

#### Scenario: Destroying the owner of a detached component
- **WHEN** уничтожается владелец отсоединённой панели
- **THEN** панель и её визуальные потомки уничтожаются, несмотря на отсутствие Parent у панели

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

### Requirement: Minimal logical component API for MVP
MVP SHALL предоставлять ui.NewForm(), ui.NewEdit(owner), ui.NewButton(owner), ui.NewLabel(owner), form.SetTitle(string), control.Text() string, control.SetText(string) и control.SetParent(parent) error. Конструкторы SHALL возвращать логические объекты без native-создания и автоматического Parent; native-ошибки запуска SHALL возвращаться из Run. Destroy и Destroyed сохраняют прежний контракт.

#### Scenario: Configure and attach MVP controls
- **WHEN** в main создают форму и три контрола с Owner = форма, задают текст и отдельно присоединяют их к форме
- **THEN** до Run native-ресурсы не создаются; после успешного запуска отображаются подготовленные значения

#### Scenario: Native creation fails during startup
- **WHEN** логические объекты созданы, но native-контрол не удалось создать в Run
- **THEN** ошибку сообщает Run с очисткой частично созданных ресурсов по application-failure

### Requirement: Fixed pre-run geometry for MVP
MVP SHALL предоставлять form.SetClientSize(width, height) и control.SetBounds(x, y, width, height) с целыми логическими единицами только до Run. ClientSize SHALL задавать точный размер клиентской области без декораций. Координаты Bounds SHALL быть фиксированы относительно внутренней области Parent. Размеры SHALL быть положительными, координаты неотрицательными; нарушение значений SHALL вызывать panic. Автоматическое размещение не выполняется.

#### Scenario: Fixed form layout
- **WHEN** до Run задана клиентская область 400 на 180 и координаты контролов внутри неё
- **THEN** backend сохраняет заданные координаты и точный размер клиентской области в своём координатном пространстве с учётом начального масштаба; размер каждого контрола не меньше заданного и может быть увеличен нативной темой, без автоматической перестановки контролов

#### Scenario: Invalid dimensions
- **WHEN** до Run размер задан нулём или отрицательным числом либо координата отрицательна
- **THEN** setter вызывает panic как ошибку использования API

### Requirement: Fixed window size and startup scale for MVP
Главная форма MVP MUST NOT допускать пользовательское resizing. Масштаб SHALL фиксироваться при запуске; перенос между мониторами с разным DPI и изменение геометрии во время работы не входят в поддерживаемый MVP. Одинаковые Bounds MUST NOT трактоваться как обещание одинакового внешнего вида разных native controls.

#### Scenario: User attempts to resize
- **WHEN** пользователь пытается изменить размер главного окна штатными средствами resizing
- **THEN** пользовательское изменение размера недоступно, заданная клиентская область сохраняется при исходном масштабе

### Requirement: Native minimum control sizes preserve appearance in MVP
Размеры SetBounds SHALL быть нижними границами, а не требованием сжатия нативного контрола ниже минимума темы. Golphi MUST NOT менять оформление темы ради уменьшения контрола до заданного минимального размера. Автоматическое устранение пересечений и layout этим не вводятся.

#### Scenario: Theme requires a taller control
- **WHEN** для Edit задана высота 28, но текущая нативная тема требует 29
- **THEN** backend может выделить высоту 29, сохраняя заданные координаты, нативный вид и точный размер клиентской области формы
