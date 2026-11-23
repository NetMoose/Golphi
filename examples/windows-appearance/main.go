// Windows-appearance demonstrates manual bounds with native controls.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/NetMoose/Golphi/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Golphi:", err)
		os.Exit(1)
	}
}

func run() error {
	form := ui.NewForm()
	form.SetTitle("Golphi — оформление контролов")
	form.SetClientSize(544, 370)

	const sample = "Й и ё, Д и д, Ц и ц, Щ и щ"
	label := ui.NewLabel(form)
	label.SetText(sample)
	label.SetBounds(16, 16, 512, 28)

	edit := ui.NewEdit(form)
	edit.SetText(sample)
	edit.SetBounds(16, 52, 320, 34)

	longEdit := ui.NewEdit(form)
	longEdit.SetText(sample + " — длинная строка для горизонтальной прокрутки. Конец строки.")
	longEdit.SetBounds(16, 98, 320, 34)

	button := ui.NewButton(form)
	button.SetText("Добавить подпись")
	button.SetBounds(16, 144, 200, 36)

	heading := ui.NewLabel(form)
	heading.SetText("Тесные контролы и края панелей")
	heading.SetBounds(16, 200, 512, 28)

	outer := ui.NewPanel(form)
	outer.SetBounds(16, 234, 490, 120)

	tightButton := ui.NewButton(outer)
	tightButton.SetText(sample)
	tightButton.SetBounds(8, 4, 56, 34)

	tightEdit := ui.NewEdit(outer)
	tightEdit.SetText(sample)
	tightEdit.SetBounds(180, 4, 56, 34)

	tightLabel := ui.NewLabel(outer)
	tightLabel.SetText(sample)
	tightLabel.SetBounds(348, 8, 56, 28)

	inner := ui.NewPanel(outer)
	inner.SetBounds(8, 56, 226, 48)
	innerEdge := ui.NewLabel(inner)
	innerEdge.SetText("Край вложенной панели — " + sample)
	innerEdge.SetBounds(130, 8, 360, 28)

	outerEdge := ui.NewLabel(outer)
	outerEdge.SetText("Край внешней панели — " + sample)
	outerEdge.SetBounds(410, 64, 360, 28)

	if err := errors.Join(
		label.SetParent(form), edit.SetParent(form), longEdit.SetParent(form),
		button.SetParent(form), heading.SetParent(form),
		tightButton.SetParent(outer), tightEdit.SetParent(outer), tightLabel.SetParent(outer),
		innerEdge.SetParent(inner), inner.SetParent(outer), outerEdge.SetParent(outer), outer.SetParent(form),
	); err != nil {
		return fmt.Errorf("подготовка окна: %w", err)
	}

	var dynamicLabel *ui.Label
	button.SetOnClick(func() {
		if dynamicLabel == nil {
			dynamicLabel = ui.NewLabel(form)
			dynamicLabel.SetText(sample)
			dynamicLabel.SetBounds(232, 148, 296, 28)
		}

		var parent ui.Container = form
		caption := "Скрыть подпись"
		if dynamicLabel.Parent() != nil {
			parent = nil
			caption = "Вернуть подпись"
		}
		if err := dynamicLabel.SetParent(parent); err != nil {
			button.SetText("Ошибка операции")
			fmt.Fprintln(os.Stderr, err)
			return
		}
		button.SetText(caption)
	})

	return form.Run()
}
