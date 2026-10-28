//go:build linux && amd64

// smoke-gtk exercises the native driver on the process startup thread.
package main

import (
	"errors"
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/backend/gtk4"
	"github.com/NetMoose/Golphi/internal/uithread"
	"os"
	"time"
)

func run() (err error) {
	if !uithread.IsUI() {
		return fmt.Errorf("not on startup thread")
	}
	d := gtk4.New()
	defer func() {
		err = errors.Join(err, d.Close())
		if err == nil {
			fmt.Printf("GTK native startup-thread smoke passed: startup=%d current=%d unicode=true change=true goroutine_wake=true cleanup=true\n", uithread.Startup(), uithread.Current())
		}
	}()
	changes := 0
	if err = d.Init(backend.Callbacks{Event: func(e backend.Event) {
		if !uithread.IsUI() {
			panic("callback left startup thread")
		}
		if e.Kind == backend.Change && e.Text == "Изменено 🌍" {
			changes++
		}
	}}); err != nil {
		return err
	}
	form, err := d.Create(backend.Config{Kind: backend.Form, Text: "GTK smoke", Bounds: backend.Bounds{Width: 400, Height: 220}})
	if err != nil {
		return err
	}
	edit, err := d.Create(backend.Config{Kind: backend.Edit, Parent: form, Text: "Привет", Bounds: backend.Bounds{X: 20, Y: 20, Width: 300, Height: 40}})
	if err != nil {
		return err
	}
	if changes != 0 {
		return fmt.Errorf("construction event")
	}
	if err = d.Show(form); err != nil {
		return err
	}
	if err = d.Show(edit); err != nil {
		return err
	}
	q := backend.NewQueue(d.Wake)
	q.Open()
	defer q.Stop()
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = q.Post(func() {
			if err := d.SetText(edit, "Изменено 🌍"); err != nil {
				panic(err)
			}
			d.Quit()
		})
	}()
	p, err := d.Run(q.Dispatch)
	if err != nil {
		return err
	}
	if p != nil {
		return fmt.Errorf("callback panic: %v", p.Value)
	}
	text, err := d.Text(edit)
	if err != nil {
		return err
	}
	if changes != 1 || text != "Изменено 🌍" {
		return fmt.Errorf("changes=%d text=%q", changes, text)
	}
	if err = d.Destroy(edit); err != nil {
		return err
	}
	if err = d.Destroy(form); err != nil {
		return err
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
