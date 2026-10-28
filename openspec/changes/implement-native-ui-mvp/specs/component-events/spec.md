# Spec Delta

## ADDED Requirements

### Requirement: MVP event handler signatures
MVP SHALL предоставлять button.SetOnClick(func()) и edit.SetOnChange(func()). Они SHALL сохранять правила одного обработчика, замены и nil. OnChange SHALL соблюдать прежние правила реального изменения, тихой подготовки, отложенного объединения, отмены и сброса базы; native-сигнал и SetText MUST NOT дублировать одно изменение.

#### Scenario: Closure reads current text
- **WHEN** пользователь нажимает кнопку, обработчик func() читает edit.Text() и меняет label.SetText()
- **THEN** подпись отражает введённый текст, в том числе кириллицу, без параметра sender в обработчике

#### Scenario: Programmatic change notified once
- **WHEN** SetText меняет текст работающего присоединённого Edit и backend сообщает об этом native-сигналом
- **THEN** прикладной OnChange вызывается по единому контракту изменения без дублирования уведомления

### Requirement: Normal MVP shutdown cancels pending notifications
При начале обычной остановки MVP Golphi SHALL отменять все ещё не начатые прикладные уведомления, включая ожидающий OnChange, и MUST NOT запускать новые прикладные обработчики. Уже выполняющийся вызов SHALL завершаться без принудительного прерывания; отменённые уведомления MUST NOT восстанавливаться во время очистки.

#### Scenario: Close with queued text change
- **WHEN** запрос закрытия обработан при ожидающем повторном OnChange
- **THEN** уведомление отменяется и не доставляется при очистке
