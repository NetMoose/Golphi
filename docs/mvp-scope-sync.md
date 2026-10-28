# Исторический снимок Scope до синхронизации MVP

Здесь сохранён переходный снимок Scope, подготовленный до синхронизации пяти MVP-дельт. Это историческая запись, а не шаблон для повторного применения и не описание текущего состояния спецификаций. Текущие Scope и Requirements находятся в основных спецификациях: [application-failure](../openspec/specs/application-failure/spec.md), [post-dispatch](../openspec/specs/post-dispatch/spec.md), [component-events](../openspec/specs/component-events/spec.md), [native-ui-runtime](../openspec/specs/native-ui-runtime/spec.md) и [component-lifecycle](../openspec/specs/component-lifecycle/spec.md). Исходные дельты сохранены в [архиве MVP](../openspec/changes/archive/2026-10-28-implement-native-ui-mvp/proposal.md).

Тексты в блоках ниже сохранены без обновления: упоминания активного изменения и несинхронизированных Requirements относятся к состоянию до sync/archive. Относительные пути внутри блоков показаны как исторический исходный текст Scope и не являются действующими ссылками этого документа. Открытые вопросы сохраняются.


## application-failure

Текст для `openspec/specs/application-failure/spec.md` (ссылки относительно этого файла):

```markdown
Контракт касается паники прикладного обработчика или функции Post, прекращения прикладной доставки, отмены ожидающих уведомлений и всех ещё не начатых задач Post и отклонения новых Post при аварийной остановке. После выхода из callback выполняются попытки очистки; ошибки и перехваченные паники отдельных операций очистки дополняют первоначальную панику, её значение и стек, не препятствуя попыткам очистить остальные независимые ресурсы. После завершения попыток Form.Run() возвращает ненулевую ошибку со всей собранной диагностикой; перехваченные паники очистки повторно наружу не выбрасываются.

Для узкого MVP обычное закрытие, ошибки запуска и очистка без первичной паники приняты в [активном изменении implement-native-ui-mvp](../../changes/implement-native-ui-mvp/specs/application-failure/spec.md); bootstrap, единственная попытка Run и состояние после возврата определены его [native-ui-runtime](../../changes/implement-native-ui-mvp/specs/native-ui-runtime/spec.md). Это переходная ссылка до отдельного sync/archive: Requirements ниже ещё не синхронизированы с MVP-дельтой и не доказывают полноту реализации.

Открыты самостоятельный Destroy, сбои внутренних механизмов вне отдельных операций очистки, порядок очистки зависимых ресурсов и определение зависимостей, повторные попытки и судьба неосвобождённых ресурсов, зависшие операции, тайм-ауты и принудительное прерывание, стабильный публичный тип ошибки, API доступа, представление стеков, способ объединения и порядок представления диагностики. Обычная остановка, запуск, повторный Run и состояние после возврата полной библиотеки вне выбранного MVP не определены. Остальные вопросы E0 из [дизайна очистки](../../changes/archive/2026-10-27-define-cleanup-failure-contract/design.md) сохраняются.
```

## post-dispatch

Текст для `openspec/specs/post-dispatch/spec.md` (ссылки относительно этого файла):

```markdown
Контракт относится к обычному Post(fn), без владельца. Очистка и диагностика при аварийной остановке подчиняются [application-failure](../application-failure/spec.md): первоначальная паника сохраняется, сбои очистки добавляются к диагностике, попытки очистить остальные независимые ресурсы продолжаются, а Form.Run() возвращает всю собранную диагностику после завершения попыток.

Для узкого MVP пакетный ui.Post(func()) error, граница готовности и отмена при обычном закрытии приняты в [активной дельте post-dispatch](../../changes/implement-native-ui-mvp/specs/post-dispatch/spec.md); bootstrap/закрепление UI-потока, ограничения Run и проверка потока — в [native-ui-runtime](../../changes/implement-native-ui-mvp/specs/native-ui-runtime/spec.md); очистка без первичной паники — в [дельте application-failure](../../changes/implement-native-ui-mvp/specs/application-failure/spec.md). Это переходные ссылки до отдельного sync/archive: Requirements ниже ещё не синхронизированы с MVP-дельтой и не доказывают полноту реализации.

Ожидание результата, индивидуальная отмена, задачи с владельцем, окончательный получатель Post полной библиотеки, стабильные типы отказа и перегрузка, порядок относительно native events/OnChange, справедливость и взаимные изменения остаются открытыми. Сохраняются открытые вопросы очистки из application-failure, а также bootstrap, готовность, потоковые правила и обычная остановка полной библиотеки вне выбранного MVP. Остальные вопросы E0 из [дизайна очистки](../../changes/archive/2026-10-27-define-cleanup-failure-contract/design.md) этим контрактом не разрешаются.
```

## component-events

Текст для `openspec/specs/component-events/spec.md` (ссылки относительно этого файла):

```markdown
Для правил событий визуальный контрол присоединён, если цепочка его Parent достигает формы. Наличие непосредственного Parent само по себе недостаточно. Отсоединение любого предка делает всё его поддерево отсоединённым; Owner и видимость не определяют присоединённость. Работающее состояние является отдельным условием доставки событий.

Правила отложенной повторной доставки определены требованиями «Deferred and coalesced repeated changes», «Destruction cancels unstarted component notifications», «Detachment cancels pending text notifications», «Pending text notification follows the current handler», «Disabling cancels queued and accumulated text notifications» и «Comparison restarts after disabling» ниже; паника обработчика и аварийная остановка определены [спецификацией application-failure](../application-failure/spec.md). Принятие, выполнение и отмена обычных задач Post описаны [спецификацией post-dispatch](../post-dispatch/spec.md).

Для узкого MVP сигнатуры button.SetOnClick(func()) / edit.SetOnChange(func()) и отмена уведомлений при обычном закрытии приняты в [активной дельте component-events](../../changes/implement-native-ui-mvp/specs/component-events/spec.md). Это переходная ссылка до отдельного sync/archive: Requirements ниже ещё не синхронизированы с MVP-дельтой и не доказывают полноту реализации. Сигнатуры других событий и Program/User, обычная остановка полной библиотеки вне MVP, порядок относительно Post/native events, взаимные изменения компонентов, новое накопление после отсоединения/присоединения внутри одного callback и прочие неописанные сочетания остаются открытыми.
```
