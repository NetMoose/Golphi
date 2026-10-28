// Hello demonstrates the same public UI API on every supported platform.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NetMoose/Golphi/ui"
)

func main() {
	form := ui.NewForm()
	form.SetTitle("Golphi — приветствие")
	form.SetClientSize(400, 180)

	nameEdit := ui.NewEdit(form)
	nameEdit.SetText("Мир")
	nameEdit.SetBounds(16, 16, 368, 28)
	if err := nameEdit.SetParent(form); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	greetButton := ui.NewButton(form)
	greetButton.SetText("Поздороваться")
	greetButton.SetBounds(16, 56, 160, 32)
	if err := greetButton.SetParent(form); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	resultLabel := ui.NewLabel(form)
	resultLabel.SetText("Введите имя и нажмите кнопку.")
	resultLabel.SetBounds(16, 104, 368, 32)
	if err := resultLabel.SetParent(form); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	var request uint64 // Accessed only on the UI thread.
	greetButton.SetOnClick(func() {
		name := strings.TrimSpace(nameEdit.Text())
		if name == "" {
			name = "мир"
		}
		request++
		current := request
		resultLabel.SetText("Привет, " + name + "! Работаю…")

		// Capture the input on the UI thread. The worker uses ordinary Go data;
		// even reading a live control belongs in the handler or a Post callback.
		go func() {
			time.Sleep(2 * time.Second) // Simulate slow work without blocking input.
			result := fmt.Sprintf("Готово, %s! Символов: %d", name, utf8.RuneCountInString(name))
			if err := ui.Post(func() {
				if current == request && !resultLabel.Destroyed() {
					resultLabel.SetText(result)
				}
			}); err != nil {
				// Closing the window can stop admission before this worker finishes.
				fmt.Fprintln(os.Stderr, "Результат не передан в UI:", err)
			}
		}()
	})

	// Run belongs on the startup thread and may be attempted only once.
	if err := form.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Golphi:", err)
		os.Exit(1)
	}
}
