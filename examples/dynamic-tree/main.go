// Dynamic-tree uses the same public UI API on Linux and Windows.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/NetMoose/Golphi/ui"
)

func main() {
	form := ui.NewForm()
	form.SetTitle("Golphi — динамическое дерево")
	form.SetClientSize(1000, 680)

	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	label := func(owner ui.Component, parent ui.Container, text string, x, y, w, h int) *ui.Label {
		l := ui.NewLabel(owner)
		l.SetText(text)
		l.SetBounds(x, y, w, h)
		must(l.SetParent(parent))
		return l
	}
	button := func(text string, x, y int, click func()) {
		b := ui.NewButton(form)
		b.SetText(text)
		b.SetBounds(x, y, 470, 36)
		b.SetOnClick(click)
		must(b.SetParent(form))
	}
	status := label(form, form, "Готово. Поддерево слева; подробная проверка — docs/dynamic-tree-example.md.", 16, 568, 968, 30)
	events := label(form, form, "OnChange поддерева: 0", 16, 604, 968, 30)
	report := func(err error, success string) bool {
		if err != nil {
			status.SetText("Ошибка операции: " + err.Error())
			fmt.Fprintln(os.Stderr, err)
			return false
		}
		status.SetText(success)
		return true
	}
	label(form, form, "Внешнее поле: проверка отсутствия захвата фокуса", 16, 16, 968, 26)
	outside := ui.NewEdit(form)
	outside.SetText("Фокус вне переносимого поддерева")
	outside.SetBounds(16, 48, 968, 32)
	must(outside.SetParent(form))
	label(form, form, "ЛЕВАЯ ПАНЕЛЬ", 16, 278, 470, 26)
	label(form, form, "ПРАВАЯ ПАНЕЛЬ", 514, 278, 470, 26)
	left, right := ui.NewPanel(form), ui.NewPanel(form)
	left.SetBounds(16, 308, 470, 240)
	right.SetBounds(514, 308, 470, 240)
	must(left.SetParent(form))
	must(right.SetParent(form))

	// All mutable state below belongs to the UI thread, including Post callbacks.
	var tree *ui.Panel
	var edit *ui.Edit
	var slots [2]*ui.Label
	var changes, creations int
	var request uint64
	var destination ui.Container = left
	create := func() {
		if tree != nil && !tree.Destroyed() {
			status.SetText("Поддерево уже существует; сначала уничтожьте его.")
			return
		}
		request++
		tree = ui.NewPanel(form) // Owner stays Form even when Parent changes.
		tree.SetBounds(12, 12, 430, 214)
		edit = ui.NewEdit(tree)
		edit.SetText("АБ😀вгД — выделите часть текста")
		edit.SetBounds(8, 8, 406, 34)
		edit.SetOnChange(func() {
			changes++
			events.SetText(fmt.Sprintf("OnChange поддерева: %d", changes))
		})
		must(edit.SetParent(tree))
		nested := ui.NewPanel(tree)
		nested.SetBounds(8, 52, 260, 42)
		// This deliberately overflows the nested panel, not the window.
		label(nested, nested, "Край: 0123456789 АБВГДЕЖЗ — ЭТОТ ХВОСТ СКРЫТ", 180, 4, 420, 32)
		must(nested.SetParent(tree))
		label(tree, tree, "← clipping: текст обрезан у x=260", 8, 100, 406, 26)
		slots = [2]*ui.Label{}
		destination = left
		report(tree.SetParent(destination), "Создано новое поддерево слева. OnChange от подготовки не ожидается.")
	}
	alive := func() bool {
		if tree == nil || tree.Destroyed() {
			status.SetText("Поддерево уничтожено. Нажмите «Создать заново».")
			return false
		}
		return true
	}
	addChild := func(slot int, source string) {
		if !alive() {
			return
		}
		if slots[slot] != nil {
			slots[slot].Destroy()
		}
		creations++
		slots[slot] = label(tree, tree, fmt.Sprintf("Ребёнок %d: %s", creations, source), 8, 132+slot*34, 406, 30)
		status.SetText("Ребёнок создан в " + source + "; повтор заменяет только свою строку.")
	}
	// A worker only waits and posts. It never reads or changes UI objects.
	later := func(message string, action func()) {
		request++
		current := request
		target := tree
		status.SetText(message)
		go func() {
			time.Sleep(8 * time.Second)
			if err := ui.Post(func() {
				if current != request || target != tree || target == nil || target.Destroyed() {
					return
				}
				action()
			}); err != nil {
				fmt.Fprintln(os.Stderr, "Отложенная операция не принята:", err)
			}
		}()
	}
	button("Создать ребёнка в обработчике", 16, 92, func() { addChild(0, "OnClick") })
	button("Создать ребёнка через Post", 514, 92, func() {
		if !alive() {
			return
		}
		target := tree
		if err := ui.Post(func() {
			if tree == target && !target.Destroyed() {
				addChild(1, "Post")
			}
		}); err != nil {
			report(err, "")
		}
	})
	button("Перенести через 8 с — верните фокус в поле", 16, 136, func() {
		if !alive() {
			return
		}
		later("Через 8 с: прямой перенос. Сейчас поставьте курсор или выделите текст.", func() {
			if tree.Parent() == nil {
				status.SetText("Перенос отменён: поддерево отсоединено.")
				return
			}
			var next ui.Container = right
			if tree.Parent() == right {
				next = left
			}
			if report(tree.SetParent(next), "Прямой перенос выполнен. Проверьте ввод без щелчка мышью.") {
				destination = next
			}
		})
	})
	button("Отсоединить поддерево", 514, 136, func() {
		if !alive() {
			return
		}
		request++
		report(tree.SetParent(nil), "Поддерево отсоединено; текст сохранён. Можно тихо настроить или присоединить.")
	})
	button("Тихая настройка отсоединённого дерева", 16, 180, func() {
		if !alive() {
			return
		}
		if tree.Parent() != nil {
			status.SetText("Сначала отсоедините поддерево.")
			return
		}
		edit.SetText("Тихо подготовлено: Я😀Б — новая база")
		edit.SetBounds(8, 8, 396, 34)
		status.SetText("Текст и Bounds изменены без OnChange. Присоедините дерево.")
	})
	button("Присоединить через 8 с", 514, 180, func() {
		if !alive() {
			return
		}
		if tree.Parent() != nil {
			status.SetText("Дерево уже присоединено.")
			return
		}
		later("Через 8 с: присоединение. Поставьте фокус во внешнее поле.", func() {
			report(tree.SetParent(destination), "Дерево присоединено. Текст сохранён; фокус не должен перейти в него.")
		})
	})
	button("Уничтожить поддерево", 16, 224, func() {
		if !alive() {
			return
		}
		request++
		tree.Destroy()
		status.SetText("Поддерево и его дети уничтожены. Теперь можно создать новое.")
	})
	button("Создать заново", 514, 224, create)
	create()
	if err := form.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Golphi:", err)
		os.Exit(1)
	}
}
