# Spec Delta

## ADDED Requirements

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
