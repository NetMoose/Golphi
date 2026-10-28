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
MVP SHALL предоставлять form.SetClientSize(width, height) и control.SetBounds(x, y, width, height) с целыми логическими единицами. ClientSize SHALL исключать декорации окна; Bounds SHALL отсчитываться от внутренней области Parent. Эти методы разрешены только до Run. Размеры SHALL быть положительными, координаты неотрицательными; нарушение значений SHALL вызывать panic. Автоматическое размещение не выполняется.

#### Scenario: Fixed form layout
- **WHEN** до Run задана клиентская область 400 на 180 и координаты контролов внутри неё
- **THEN** backend отображает заданную геометрию в своём координатном пространстве с учётом начального масштаба, без автоматической перестановки контролов

#### Scenario: Invalid dimensions
- **WHEN** до Run размер задан нулём или отрицательным числом либо координата отрицательна
- **THEN** setter вызывает panic как ошибку использования API

### Requirement: Fixed window size and startup scale for MVP
Главная форма MVP MUST NOT допускать пользовательское resizing. Масштаб SHALL фиксироваться при запуске; перенос между мониторами с разным DPI и изменение геометрии во время работы не входят в поддерживаемый MVP. Одинаковые Bounds MUST NOT трактоваться как обещание одинакового внешнего вида разных native controls.

#### Scenario: User attempts to resize
- **WHEN** пользователь пытается изменить размер главного окна штатными средствами resizing
- **THEN** пользовательское изменение размера недоступно, заданная клиентская область сохраняется при исходном масштабе
