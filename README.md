# Golphi

![Логотип Golphi](logo/golphi-icon-256.png)

Экспериментальный нативный GUI-фреймворк для Go с компонентной моделью, близкой к Delphi VCL. Текущая реализация поддерживает одну форму, Edit, Button, Label, вложенные Panel с clipping, события и передачу фоновой работы через `ui.Post`. Оба примера используют один Go API на Win32 и GTK4. Полные этапы E0 и E1 и стабилизация публичного API остаются впереди.

- [Hello](examples/hello/main.go) строит дерево до `form.Run()`.
- [Динамическое дерево](examples/dynamic-tree/main.go) показывает создание компонентов из обработчика и Post, перенос поддерева, отсоединение, подготовку к повторному присоединению и уничтожение.

Конструкторы контролов и Panel принимают обязательного неизменяемого владельца `Component`. `SetParent` принимает `Container` (Form или Panel) в пределах той же формы. Эти сигнатуры и возвращаемые типы Owner/Parent изменились после MVP. Во время Run создание, `SetParent` и `SetBounds` требуют UI-потока, включая вызовы для отсоединённого дерева и назначение того же Parent. Для вызова из worker-goroutine используется `ui.Post`. Присоединение поддерева синхронно подготавливает его к работе до возврата `SetParent`, без ожидания перерисовки.

`SetClientSize` задаётся до Run и определяет точный клиентский размер. `SetBounds` задаёт фиксированные координаты и минимальные размеры контролов, которые тема может увеличить. Во время Run bounds можно менять только у отсоединённых контролов. Layout, resizing, несколько окон, Qt, Auto, файлы форм и дизайнер находятся за пределами текущего среза.

Прямой перенос сохраняет текст и фокус Edit. Курсор, диапазон выделения и его активный конец сохраняются при переносе существующего native Edit. При пересоздании Edit восстановление позиций отложено. Отсоединение и повторное присоединение сохраняют текст, а повторное присоединение само по себе не захватывает фокус.

Сборка примеров без C toolchain. Linux требует установленного GTK4 runtime, Windows использует системные DLL.

```sh
CGO_ENABLED=0 go build -o hello ./examples/hello
CGO_ENABLED=0 go build -o dynamic-tree ./examples/dynamic-tree
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags='-H windowsgui' -o dynamic-tree.exe ./examples/dynamic-tree
```

Оба примера собраны с `CGO_ENABLED=0` для Linux и Windows amd64. Ручные сценарии динамического примера приняты на Linux/X11 GTK4 по отзыву от 13 ноября и на Windows 11 по подтверждению от 20 ноября. Замечание Linux о выделении относится к пересозданному GTK Edit. Финальное подтверждение Windows дано без нового машинного журнала. [Сборки и хеши](docs/dynamic-tree-builds.md) · [Инструкция и ручная приёмка](docs/dynamic-tree-example.md) · [Нативные проверки](docs/dynamic-tree-validation.md)

- [Экспериментальный API и ограничения](docs/components-mvp.md)
- [Текущий срез, общий дизайн и открытые вопросы](docs/design.md)
- [Активное изменение OpenSpec](openspec/changes/implement-dynamic-component-tree/proposal.md)
- [Отчёт о принятом статическом MVP](docs/mvp-validation.md) и [его инструкция сборки](docs/mvp-build.md)

Лицензия [MIT](LICENSE).

[Текущая область реализации и матрица свидетельств](docs/dynamic-tree-scope.md).
