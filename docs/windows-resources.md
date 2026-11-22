# Манифест Windows и visual styles

Манифест принадлежит приложению. Windows загружает Common Controls v6 для
приложения Golphi, когда в EXE встроен манифест с этой зависимостью. Подготовка
ресурса требует Go 1.26.5 и доступных Go-модулей. Генерация и сборка работают
с `CGO_ENABLED=0` без компилятора C и компилятора ресурсов Windows SDK.

## Примеры Golphi

В пакетах `examples/hello` и `examples/dynamic-tree` лежат исходный
`app.manifest.xml`, конфигурация `winres.json` и подготовленный
`rsrc_windows_amd64.syso`. Суффикс `_windows_amd64.syso` ограничивает
включение ресурса сборками Windows/amd64. Обычный `go build` использует
готовый ресурс без запуска генератора. Для поставки достаточно EXE.

Модуль [tools/winres](../tools/winres/go.mod) фиксирует релиз
[go-winres v0.3.3](https://github.com/tc-hib/go-winres/releases/tag/v0.3.3)
и версии зависимостей. [go.sum](../tools/winres/go.sum) хранит их контрольные
суммы. При первом запуске Go скачивает недостающие модули. Стандартные
источники Go — `proxy.golang.org` для загрузки и `sum.golang.org` для
проверки контрольных сумм. После заполнения локального кэша можно выполнить
генерацию с `GOPROXY=off`.

### Linux

Из корня Golphi запустите генератор после изменения XML или JSON.
Команда сборки показана для hello. Для второго примера используйте
`./examples/dynamic-tree` и имя `dynamic-tree.exe`.

```sh
./scripts/generate-windows-resources.sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o hello.exe ./examples/hello
```

### Windows PowerShell

Из корня Golphi в новой сессии PowerShell выполните команды ниже. Они
заново генерируют ресурсы и собирают оба примера.

```powershell
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:GOWORK = 'off'
$env:GOFLAGS = '-mod=readonly'

foreach ($example in 'hello', 'dynamic-tree') {
    go -C tools/winres tool go-winres make --in "../../examples/$example/winres.json" --out "../../examples/$example/rsrc" --arch amd64
    if ($LASTEXITCODE -ne 0) { throw "go-winres make failed for $example" }

    go build -ldflags '-H windowsgui' -o "$example.exe" "./examples/$example"
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $example" }
}
```

## Собственное приложение

1. Если манифеста ещё нет, возьмите за основу
   [app.manifest.xml примера hello](../examples/hello/app.manifest.xml).
   Измените `assemblyIdentity` приложения. В примере задан system-aware
   DPI (`dpiAware=true` и `dpiAwareness=system`) и запуск с правами
   вызывающего пользователя (`asInvoker`, `uiAccess=false`).
2. Если манифест уже есть, добавьте в его `assembly` зависимость ниже
   или обновите существующую запись Common Controls. Сохраните остальные
   настройки приложения, включая DPI, права, совместимость и другие зависимости.
3. Поместите XML и [winres.json](../examples/hello/winres.json) в каталог
   пакета `main` или рядом с другими исходниками ресурсов приложения.
   Если имя XML отличается, укажите его в `winres.json` относительно самого
   JSON-файла. Если приложение уже собирает Windows-ресурсы, добавьте или
   обновите запись `RT_MANIFEST` → `#1` → `0409` в существующей
   конфигурации, сохранив иконки и прочие ресурсы. В пакете `main` должен
   остаться единственный встраиваемый манифест приложения.
4. Выполните `go -C <путь-к-Golphi>/tools/winres tool go-winres make --in <абсолютный-путь-к-winres.json> --out <абсолютный-путь-к-пакету-main>/rsrc --arch amd64`
   с `CGO_ENABLED=0`, `GOWORK=off` и `GOFLAGS=-mod=readonly`.
   Используйте `GOOS` и `GOARCH` хоста для запуска генератора.
   Параметр `--arch amd64` задаёт архитектуру ресурса.
   Замените старый `.syso`, если он содержал этот манифест, на ресурс,
   сгенерированный из объединённой конфигурации.
5. Соберите приложение с `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`.
   Для GUI EXE используйте `-ldflags="-H windowsgui"`.

Зависимость Common Controls для шага 2

```xml
<dependency>
  <dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
        version="6.0.0.0" processorArchitecture="*"
        publicKeyToken="6595b64144ccf1df" language="*"/>
  </dependentAssembly>
</dependency>
```

## Проверка ресурса

Для проверки собранного файла извлеките ресурсы тем же закреплённым
инструментом. Используйте абсолютный путь к EXE и отдельный каталог для
результата. В извлечённом `winres.json` проверьте единственный
`RT_MANIFEST` с ID `#1` и языком `0409`, а в соответствующем XML —
версию Common Controls `6.0.0.0` и сохранённые настройки приложения.

```text
go -C tools/winres tool go-winres extract --xml-manifest --dir <абсолютный-каталог-результата> <абсолютный-путь-к-exe>
```

Документация формата [Microsoft Application manifests](https://learn.microsoft.com/en-us/windows/win32/sbscs/application-manifests)
и [go-winres v0.3.3](https://github.com/tc-hib/go-winres/tree/v0.3.3#manifest).

XML и JSON примеров закреплены в LF через `.gitattributes`, а `.syso` —
как бинарные файлы. Это сохраняет байты манифеста и хеш ресурса при checkout
на Windows с `core.autocrlf`; при передаче ZIP исходные байты также сохраняются.
