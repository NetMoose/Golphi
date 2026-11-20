# GTK4/X11: эксперимент динамического дерева

Это отдельная native-проверка, не реализация Panel/Driver. Запуск:

```sh
CGO_ENABLED=0 GDK_BACKEND=x11 DISPLAY=:0 go run ./cmd/probe-tree-linux
GOLPHI_TREE_PROBE=1 CGO_ENABLED=0 GDK_BACKEND=x11 DISPLAY=:0 go test -count=3 -v ./cmd/probe-tree-linux
```

Нужны GTK4 (`libgtk-4.so.1`) и доступ к настоящему X11. В обычном `go test ./...` native-тест пропускается. На остальных ОС команда собирается как unsupported stub.

Проверяемый кандидат: GtkScrolledWindow с обеими policy=GTK_POLICY_EXTERNAL (3), внутри GtkFixed. Панели вложены, размер задаётся wrapper. GtkFixed сам по себе не является доказательством отсутствия роста. При добавлении огромного GtkDrawingArea сравниваются реальные allocation обеих панелей с исходными и запрошенными размерами. GtkWidget.pick проверяет доступность ребёнка внутри границы и недоступность за границей вложенной панели. Отрицательная точка outer(155,80) не перекрыта sibling: вторая панель создаётся только после clipping проверки. Огромный GtkDrawingArea рисует сплошной magenta. Native GtkWidgetPaintable → GtkSnapshot → GskRenderer.render_texture → GdkTexture.download сравнивает baseline и изображение после добавления ребёнка. PNG сохраняются в /tmp/golphi-tree-linux-before.png и /tmp/golphi-tree-linux-after.png; проверяются изменённые pixels внутри панели и ноль изменённых pixels вне неё. Это rendering GTK scene graph, не compositor screenshot.

На текущем X11 получили:

```text
clip_pixels=true inside_changed=1200 outside_changed=0
outer=260x180 inner=140x90 huge=1200x600 no_growth=true clip_pick=true
adjustments_initial=[0 0 0 0 0 0] adjustments_final=[0 0 0 0 0 0] no_auto_scroll=true fixed_coordinates=true replacement_text="А😀Б𐐷В"
move bounds=1:4 caret=4 focused=true
selection=1:4 caret=4 text="А😀Б𐐷В" move_state=true recreate_state=true callbacks=1 callback_same_thread=true disconnect=true finalized=2
```

Проверяются перенос существующего GtkEntry между двумя панелями одной формы и создание нового GtkEntry из снимка текста/позиции/выделения, включая native readback текста нового объекта. GTK offsets — символы Unicode, не UTF-8 bytes: строка имеет 5 scalar values и 14 байт, selection [1,4) включает оба non-BMP символа. Активный конец берётся из gtk_editable_get_position, второй конец восстанавливается из bounds. Фокус проверяется через потомка GtkEntry: GTK фокусирует внутренний GtkText.

Обнаружен риск: grab_focus и немедленный select_region до обработки main loop давали несовпадение состояния после переноса или recreate. В эксперименте сначала обрабатывается main loop, потом восстанавливается выделение. Задержка 150 мс — только средство эксперимента, не допустимый production протокол. Нужен событийный механизм завершения смены фокуса. Проверены выбранный диапазон и caret одновременно; отдельная матрица collapsed caret, IME composition, grapheme clusters, обратное выделение и реальные клавиатурные события остаются открытыми.

Startup OS thread закреплён internal/uithread. changed callback сравнивает текущий TID со startup TID и содержит panic boundary. После disconnect изменение текста не вызывает callback. Weak notifications подтверждают финализацию обоих entry после remove/unref. Window уничтожается, GTK остаётся загруженной до выхода процесса из-за глобальных TLS/handlers. Это не leak-профилирование, не доказательство очистки всех внутренних объектов GTK. Всего три process-lifetime callback trampoline (changed, weak notification, draw), не callback на каждый объект. Main-loop drain ограничен deadline; native subprocess имеет timeout 20 секунд и завершает process group при зависании.

На каждом автоматически созданном GtkViewport отключено scroll-to-focus. До и после фокусировки/переноса/recreate сравниваются шесть horizontal/vertical adjustments: все равны нулю; native GtkFixed positions второй панели (160,10) и большого ребёнка (100,60) сохраняются. EXTERNAL policy сама по себе не запрещает auto-reveal: явное отключение scroll-to-focus обязательно для этого кандидата.

Нужны дальнейшие проверки: отрицательные координаты, глубокая вложенность, resize, темы/DPI, скрытые панели, другие источники изменения adjustments (wheel/touch/программная прокрутка), массовые циклы создания/удаления. Результат одной установленной GTK/X11 не гарантирует поведение всех версий/тем.

Context7 resolve GTK4 завершился transport error. Использованы официальные документы:

- [GtkFixed](https://docs.gtk.org/gtk4/class.Fixed.html)
- [GtkScrolledWindow и GtkViewport](https://docs.gtk.org/gtk4/class.ScrolledWindow.html)
- [GtkEditable.select_region: character offsets](https://docs.gtk.org/gtk4/method.Editable.select_region.html)

- [GtkViewport.set_scroll_to_focus](https://docs.gtk.org/gtk4/method.Viewport.set_scroll_to_focus.html)
- [GskRenderer.render_texture](https://docs.gtk.org/gsk4/method.Renderer.render_texture.html)
