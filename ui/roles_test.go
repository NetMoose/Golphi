package ui

import "testing"

// Catch loss of general ownership, visual container roles, or Panel's minimal API.
func TestGeneralComponentRoles(t *testing.T) {
	f := NewForm()
	a, b := NewPanel(f), NewPanel(f)
	button := NewButton(a)
	if err := button.SetParent(b); err != nil {
		t.Fatal(err)
	}
	if button.Owner() != a || button.Parent() != b {
		t.Fatal("owner and parent must be independent")
	}
	if a.Owner() != f || a.Parent() != nil || f.Owner() != nil || f.Parent() != nil {
		t.Fatal("unexpected initial graph")
	}
	a.SetBounds(1, 2, 30, 40)
	if _, ok := any(a).(interface{ Text() string }); ok {
		t.Fatal("Panel exposes text")
	}
	if _, ok := any(a).(interface{ SetText(string) }); ok {
		t.Fatal("Panel exposes text setter")
	}
	leaf := NewLabel(button)
	if leaf.Owner() != button {
		t.Fatal("a non-container must be a valid owner")
	}
}

// Catch invalid owners being accepted or consuming component registrations.
func TestGeneralOwnerValidation(t *testing.T) {
	var form *Form
	var panel *Panel
	dead := NewPanel(NewForm())
	dead.Destroy()
	for _, owner := range []Component{nil, form, panel, dead} {
		for _, construct := range []func(Component){func(c Component) { NewEdit(c) }, func(c Component) { NewButton(c) }, func(c Component) { NewLabel(c) }, func(c Component) { NewPanel(c) }} {
			before := nextComponentOrder
			func() {
				defer func() {
					if recover() == nil {
						t.Error("invalid owner accepted")
					}
				}()
				construct(owner)
			}()
			if nextComponentOrder != before {
				t.Fatal("invalid owner registered a component")
			}
		}
	}
}
