# Spec Delta

## Purpose

Определить проверяемый запуск нативного интерфейса Golphi, UI-поток и завершение приложения в ограниченном первом MVP.

## ADDED Requirements

### Requirement: Native MVP platform matrix
MVP SHALL запускать один исходный пример с нативными Win32 controls на Windows 11/amd64 и GTK4 widgets на Linux Mint 22.3/amd64, glibc 2.39, GTK 4.14.5, GLib 2.80.0, Cinnamon/X11. Обе сборки SHALL использовать Go 1.26.5 и CGO_ENABLED=0 без C toolchain, GUI headers и pkg-config; Linux-запуск требует GTK4 runtime.

#### Scenario: Build and run are separate evidence
- **WHEN** одинаковый пример собирают для двух целевых ОС и запускают на каждой из согласованных конфигураций
- **THEN** сборки не требуют C toolchain, реальные запуски подтверждают соответствующие нативные контролы; одна кросс-компиляция не считается проверкой запуска

### Requirement: Startup thread owns native UI
Golphi SHALL закреплять startup thread внутри UI-пакета через init без загрузки native backend и создания окна. Form.Run SHALL вызываться из main или обычного синхронного помощника на этом потоке. Native-инициализация, создание, обработчики, функции Post и очистка SHALL выполняться на том же UI-потоке.

#### Scenario: Logical preparation before Run
- **WHEN** программа импортировала UI-пакет и настраивает форму до Run
- **THEN** startup thread закреплён, а native backend и окно ещё не созданы

#### Scenario: Background work returns to UI
- **WHEN** обработчик запускает фоновую goroutine, отправляющую допустимую задачу через Post
- **THEN** дошедшая до выполнения задача обновляет UI на потоке native-инициализации

### Requirement: One validated Run attempt per process
MVP SHALL поддерживать одну главную форму и одну прошедшую проверки попытку Run на процесс. Вызов Run живой формы вне UI-потока либо после расходования попытки SHALL возвращать ошибку без native-действий и изменения работающего приложения. Отклонение проверкой MUST NOT расходовать попытку; после прохождения проверок попытка SHALL считаться использованной даже при ошибке инициализации.

#### Scenario: Invalid call before a valid attempt
- **WHEN** фоновая goroutine вызывает Run живой формы до корректного вызова из main
- **THEN** первый вызов возвращает ошибку без расхода попытки; корректный вызов ещё разрешён

#### Scenario: Reentrant Run
- **WHEN** прикладной обработчик повторно вызывает Run живой формы
- **THEN** вызов возвращает ошибку, текущий цикл событий продолжает работу без аварийной остановки

#### Scenario: No retry after startup failure or exit
- **WHEN** первая разрешённая попытка завершилась ошибкой инициализации либо приложение уже завершилось и Run живой формы вызывают снова
- **THEN** повторный вызов возвращает ошибку без native-инициализации

### Requirement: Destroyed form check precedes Run admission
Run логически уничтоженной формы SHALL вызывать panic до проверки UI-потока и расходования попытки, как обычная операция уничтоженного компонента. Эта проверка MUST NOT расходовать попытку или выполнять native-действия. Возврат error для неправильного Run SHALL относиться только к живой форме.

#### Scenario: Destroyed form before any attempt
- **WHEN** форму уничтожили до запуска, затем фоновая goroutine вызывает её Run
- **THEN** вызов вызывает panic, а не error неправильного потока; попытка остаётся неиспользованной и native-действия не выполняются

#### Scenario: Destroyed form after an attempt was consumed
- **WHEN** попытка Run уже израсходована и Run вызывают у логически уничтоженной формы
- **THEN** вызов вызывает panic, а не error повторного запуска, без native-действий; использованная попытка не восстанавливается

### Requirement: Main window closure stops the MVP
Обработка запроса закрытия главной формы SHALL начинать обычную остановку без возможности отмены. После выхода из текущего callback SHALL выполняться очистка на UI-потоке. Уже выполняющийся прикладной вызов MUST NOT прерываться принудительно; новые обработчики MUST NOT запускаться. При успешной очистке Run SHALL возвращать nil; сбои подчиняются application-failure.

#### Scenario: Successful normal exit
- **WHEN** пользователь закрывает главную форму и очистка проходит без сбоев
- **THEN** после выхода из callback приложение очищает ресурсы и Run возвращает nil

#### Scenario: Current invocation can finish
- **WHEN** остановка начинается при ещё выполняющемся прикладном вызове
- **THEN** вызов не прерывается принудительно, новые прикладные обработчики не запускаются, очистка ожидает выхода из callback
