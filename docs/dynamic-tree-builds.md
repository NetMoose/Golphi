# Проверка сборок динамического дерева — задача 5.2

Дата: 20 ноября 2026 года. Исходный коммит:
`d92d4ad9bd1421599cdd4a96c8ec184a57340f21`, чистая ветка `codex/dynamic-component-tree`.
Код библиотеки и примеров в этой задаче не менялся.

Хост: Linux Mint 22.3 amd64. Go: `go1.26.5 linux/amd64`, purego: `v0.11.2`,
Python для проверки PE: 3.12.3. Цели: `linux/amd64` и `windows/amd64`.
Все шесть команд завершились с кодом 0. Метаданные каждого бинарника подтвердили
`CGO_ENABLED=0`, нужные GOOS/GOARCH; метаданные примеров — исходную ревизию и
`vcs.modified=false`. Стандартный `examples/hello` также успешно собран для обеих целей.

## Проверка отсутствия зависимости от C toolchain

Для каждой платформы использован отдельный изначально пустой GOCACHE.
CC и CXX указывают на отсутствующие `/nonexistent/golphi-cc` и
`/nonexistent/golphi-cxx`. Сборка не могла воспользоваться этими компиляторами.
Модули взяты из локального кеша с GOPROXY=off. Go сообщил предупреждение о
невозможности записи служебного `.info` основного модуля в read-only кеш;
это не помешало сборкам, их коды выхода — 0. Системные компиляторы не удалялись.

Команды Linux:

```sh
set -eu
test ! -e /tmp/golphi52-linux-cache
mkdir -p /tmp/golphi52-linux-cache /tmp/golphi52/linux
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0 CC=/nonexistent/golphi-cc CXX=/nonexistent/golphi-cxx GOCACHE=/tmp/golphi52-linux-cache GOMODCACHE=/home/netmoose/work/go/pkg/mod GOPROXY=off
go env GOOS GOARCH CGO_ENABLED CC CXX GOCACHE GOMODCACHE GOPROXY
go build -o /tmp/golphi52/linux/hello ./examples/hello
go build -o /tmp/golphi52/linux/dynamic-tree ./examples/dynamic-tree
sha256sum /tmp/golphi52/linux/hello /tmp/golphi52/linux/dynamic-tree
LC_ALL=C readelf -h /tmp/golphi52/linux/hello /tmp/golphi52/linux/dynamic-tree
go version -m /tmp/golphi52/linux/hello /tmp/golphi52/linux/dynamic-tree
```

Команды Windows (cross-build на том же Linux-хосте):

```sh
set -eu
mkdir -p /tmp/golphi52/windows
mkdir /tmp/golphi52-windows-cache
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
export CC=/nonexistent/golphi-cc CXX=/nonexistent/golphi-cxx
export GOCACHE=/tmp/golphi52-windows-cache
export GOMODCACHE=/home/netmoose/work/go/pkg/mod GOPROXY=off
go version
go env GOOS GOARCH CGO_ENABLED CC CXX GOCACHE GOMODCACHE GOPROXY GOFLAGS
go build -ldflags='-H windowsgui' -o /tmp/golphi52/windows/hello.exe ./examples/hello
go build -ldflags='-H windowsgui' -o /tmp/golphi52/windows/dynamic-tree.exe ./examples/dynamic-tree
go test -c -o /tmp/golphi52/windows/win32.test.exe ./internal/backend/win32
go test -c -o /tmp/golphi52/windows/ui.test.exe ./ui
go version -m /tmp/golphi52/windows/*.exe
python3 --version
```

## Результаты и хеши

| Файл относительно `/tmp/golphi52` | Формат | SHA256 |
|---|---|---|
| `linux/hello` | ELF64 x86-64 | `710fb924ef3296a060cd10457bbf9bee0bace1b9c5ae08360b790a572443febf` |
| `linux/dynamic-tree` | ELF64 x86-64 | `bcbc3ab7c13d866381a0c33d2f97435f8925f7813d472a41b1c8a3a6fd8b3f8a` |
| `windows/hello.exe` | PE32+ AMD64, subsystem 2 | `d563dacd091bb555e7c560442e5cfe1d2d99afde2f06cd33661e827d0e6cc883` |
| `windows/dynamic-tree.exe` | PE32+ AMD64, subsystem 2 | `a5d7725502f9989cf8e777940b9c1caaa67be7858247678e1155cb7146f5c938` |
| `windows/win32.test.exe` | PE32+ AMD64, subsystem 3 | `18b007cf57fba6600aa27879478ba1ac9493e8bbb82d82ae82737981a0bbdebe` |
| `windows/ui.test.exe` | PE32+ AMD64, subsystem 3 | `cd0ee4c9ce1f35ae6a65b58be38742a9ab6e9e40618a17c535d8c11b12d75af8` |

Хеши также сохранены в [SHA256 manifest](../artifacts/dynamic-tree-builds-20261120.sha256).
Проверка заголовков ELF/PE выполнена непосредственно по созданным файлам;
PE machine = 0x8664, optional-header magic = 0x020b. Subsystem 2 означает GUI,
3 — console. Оба Windows-примера собраны с `-ldflags="-H windowsgui"`,
оба тестовых EXE — без этого флага. Команды повторной проверки:

```sh
cd /tmp/golphi52
sha256sum -c /home/netmoose/work/go/src/Golphi/artifacts/dynamic-tree-builds-20261120.sha256
go version -m linux/hello linux/dynamic-tree windows/*.exe
readelf -h linux/hello linux/dynamic-tree
```

## Границы результата

Это проверка сборки и формата бинарников, без нового запуска GUI на обеих ОС.
Она не заменяет [ручную приёмку 5.1](dynamic-tree-example.md) и прежние native-журналы.
Новые хеши относятся только к этим сборкам; старые пакеты, manifest и свидетельства
MVP сохранены. GTK4 остаётся зависимостью Linux runtime. Динамическое переключение
DPI, оформление WIN-01, остальные задачи этапа 5 и Open Questions здесь не решаются.
