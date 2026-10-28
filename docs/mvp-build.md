# Воспроизводимая сборка MVP M01

Этот сценарий проверяет **сборку**, без запуска GUI. Native-проверки Linux/Windows
фиксируются отдельно в `docs/mvp-run.md` и `docs/mvp-validation.md`.

## Требования и запуск

На Linux-хосте нужны Go **1.26.5**, Bubblewrap (`bwrap`), статический BusyBox,
`sha256sum` и разрешённые user/mount namespaces. Глобальные настройки и системные
пакеты сценарий не меняет. Кэш модулей должен заранее содержать зависимости из
`go.mod`/`go.sum`: `github.com/ebitengine/purego v0.11.2`.

Из корня проекта:

```sh
GOCACHE=/tmp/golphi-gocache \
GOMODCACHE=/tmp/golphi-gomodcache \
OUT=/tmp/golphi-mvp-build \
./scripts/build-mvp.sh
```

`OUT`, `GOCACHE` и `GOMODCACHE` принимают абсолютные пути без пробелов и специальных
символов. Значения выше используются по умолчанию. Каталог артефактов находится
в `/tmp`, вне дерева исходников.

## Что именно проверяется

Сценарий создаёт новый корень файловой системы через Bubblewrap. В нём доступны:

- `/src`: проект, только чтение;
- `/go`: выбранный `GOROOT`, только чтение;
- `/modules`: кэш модулей, только чтение;
- `/cache` и `/out`: кэш сборки и результат, запись разрешена;
- `/proof/busybox`: статический BusyBox для проверок;
- временный `/tmp`, `/proc` и минимальный `/dev`.

Хостовые `/usr`, `/bin`, `/lib`, `/include` не монтируются; сценарий проверяет их
отсутствие и отсутствие `cc`, `gcc`, `clang`, `g++`, `pkg-config` в PATH.
Системные GTK headers из `/usr/include` в этом окружении недоступны. Это проверка
отсутствия системного C toolchain и GUI headers в файловом окружении сборки,
а не только изменение PATH на обычном хосте.

Окружение очищается; выставляются `GOTOOLCHAIN=local`, `CGO_ENABLED=0`,
`GOARCH=amd64`, `GOAMD64=v1`, `GOPROXY=off`, `GOSUMDB=off`. Версия компилятора
проверяется на точное совпадение с `go1.26.5 linux/amd64`.

Команды внутри окружения для каждой цели (`GOOS=linux` / `GOOS=windows`):

```sh
go build -trimpath -buildvcs=false -o /out/hello-linux-amd64 ./examples/hello
go build -trimpath -buildvcs=false -o /out/hello-windows-amd64.exe ./examples/hello
```

Каждая цель собирается дважды, затем `cmp` проверяет побайтовое совпадение.
`-trimpath` убирает пути исходников, `-buildvcs=false` исключает изменчивую VCS
метаинформацию. `go version -m` сохраняет реальные параметры каждого бинарника.
Повторяемость проверяется для текущих исходников и зафиксированного toolchain;
это не доказательство воспроизводимости между всеми машинами и версиями Go.

## Результат

В `/tmp/golphi-mvp-build` сохраняются:

- `hello-linux-amd64`;
- `hello-windows-amd64.exe`;
- `SHA256SUMS`;
- `build.log`: версия Go, список модулей, команды, embedded build metadata,
  результаты сравнения и контрольные суммы.

Проверка сохранённых файлов:

```sh
cd /tmp/golphi-mvp-build
sha256sum -c SHA256SUMS
```

Для дополнительной проверки с пустым кэшем компиляции выберите новый каталог:

```sh
GOCACHE=/tmp/golphi-gocache/mvp-clean-proof ./scripts/build-mvp.sh
```

## Ограничения

На проверенном хосте `bwrap --unshare-all` не проходит создание сетевого namespace:
`Failed to create NETLINK_ROUTE socket: Operation not permitted`. Поэтому сценарий
изолирует user, mount, PID, UTS и IPC, но сохраняет сетевой namespace хоста.
Go работает с отключёнными module proxy и checksum database; сетевой доступ
системным namespace не блокируется. Системный C toolchain при этом отсутствует
в файловом namespace. Если Bubblewrap/user namespaces недоступны, сценарий
завершается ошибкой и не подменяет эту проверку обычной сборкой через PATH.

Успешная компиляция не подтверждает доступность GTK на runtime-хосте, отображение
окна, Win32-поведение или завершение native event loop. Эти результаты должны
иметь отдельные записи и отдельную среду выполнения.
