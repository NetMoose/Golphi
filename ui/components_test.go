package ui

import "testing"

func TestPrepareAndReparent(t *testing.T) {
	owner := NewForm()
	first, second := NewPanel(owner), NewPanel(owner)
	edit := NewEdit(owner)
	edit.SetText("Привет")
	if edit.Owner() != owner || edit.Parent() != nil {
		t.Fatal("constructor attached control")
	}
	for _, parent := range []Container{first, second, nil, first} {
		if err := edit.SetParent(parent); err != nil {
			t.Fatal(err)
		}
		if edit.Parent() != parent || edit.Owner() != owner || edit.Text() != "Привет" {
			t.Fatal("lost preparation")
		}
	}
	dead := NewForm()
	dead.Destroy()
	if err := edit.SetParent(dead); err == nil {
		t.Fatal("accepted destroyed parent")
	}
	if edit.Parent() != first || edit.Text() != "Привет" || edit.Owner() != owner {
		t.Fatal("failed attachment changed state")
	}
}

func TestFirstAttachmentFailurePreservesSettings(t *testing.T) {
	owner := NewForm()
	parent := NewPanel(owner)
	parent.Destroy()
	button := NewButton(owner)
	button.SetText("Кнопка")
	if button.SetParent(parent) == nil {
		t.Fatal("accepted destroyed parent")
	}
	if button.Parent() != nil || button.Owner() != owner || button.Text() != "Кнопка" {
		t.Fatal("failed attachment changed state")
	}
}

func TestDestroyCrossedOwnershipAndParenting(t *testing.T) {
	f := NewForm()
	a, b := NewPanel(f), NewPanel(f)
	x, y, detached := NewButton(a), NewLabel(b), NewEdit(a)
	if err := x.SetParent(b); err != nil {
		t.Fatal(err)
	}
	if err := y.SetParent(a); err != nil {
		t.Fatal(err)
	}
	a.Destroy()
	if !a.Destroyed() || !x.Destroyed() || !y.Destroyed() || !detached.Destroyed() || b.Destroyed() {
		t.Fatal("wrong cascade")
	}
	if len(b.owned) != 0 || len(b.children) != 0 {
		t.Fatal("destroyed control retained in graph")
	}
	a.Destroy()
	x.Destroy()
	b.Destroy()
}

func TestDestroyedOperationsPanic(t *testing.T) {
	form := NewForm()
	edit := NewEdit(form)
	form.Destroy()
	calls := []func(){func() { form.SetTitle("x") }, func() { form.Title() }, func() { edit.Text() }, func() { edit.SetText("x") }, func() { edit.Owner() }, func() { edit.Parent() }, func() { _ = edit.SetParent(nil) }, func() { NewButton(form) }}
	for i, call := range calls {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("operation %d did not panic", i)
				}
			}()
			call()
		}()
	}
}

func TestIndividualDestroyUnlinksBothGraphs(t *testing.T) {
	owner := NewForm()
	parent := NewPanel(owner)
	button := NewButton(owner)
	if err := button.SetParent(parent); err != nil {
		t.Fatal(err)
	}
	button.Destroy()
	button.Destroy()
	if !button.Destroyed() || len(owner.owned) != 1 || len(parent.children) != 0 {
		t.Fatal("destroy did not unlink both graphs")
	}
	owner.Destroy()
	parent.Destroy()
}

func TestAllControlsPrepareTextAndFormTitle(t *testing.T) {
	f := NewForm()
	f.SetTitle("Окно")
	if f.Title() != "Окно" {
		t.Fatal("lost title")
	}
	controls := []interface {
		SetText(string)
		Text() string
		SetParent(Container) error
		Owner() Component
		Parent() Container
	}{NewEdit(f), NewButton(f), NewLabel(f)}
	for _, c := range controls {
		c.SetText("Текст")
		if err := c.SetParent(f); err != nil {
			t.Fatal(err)
		}
		if err := c.SetParent(f); err != nil {
			t.Fatal(err)
		}
		if c.Text() != "Текст" || c.Owner() != f || c.Parent() != f {
			t.Fatal("lost control settings")
		}
	}
}
