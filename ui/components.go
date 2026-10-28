package ui

import (
	"errors"

	"github.com/NetMoose/Golphi/internal/backend"
)

// Form — логическая форма и владелец контролов экспериментального MVP.
// Создание формы не обращается к оконной системе.
type Form struct {
	component
	owned    map[*component]struct{}
	children map[*component]struct{}
}

// Edit — однострочное поле ввода.
type Edit struct{ control }

// Button — кнопка.
type Button struct{ control }

// Label — текстовая метка.
type Label struct{ control }

var nextComponentOrder uint64

type component struct {
	order         uint64
	kind          backend.Kind
	owner, parent *Form
	text          string
	eventState
	bounds backend.Bounds
	// started is latched by Run before native initialization for every participant.
	started   bool
	destroyed bool
	// form is set only for forms; graph edges refer to component identities.
	form *Form
}

type control struct{ component }

// NewForm создаёт логическую форму без native-ресурсов.
func NewForm() *Form {
	if runActive.Load() {
		panic("ui: creation during Run is unsupported")
	}
	f := &Form{owned: make(map[*component]struct{}), children: make(map[*component]struct{})}
	nextComponentOrder++
	f.order = nextComponentOrder
	f.kind, f.form = backend.Form, f
	return f
}

// NewEdit создаёт поле с владельцем, но без визуального Parent.
func NewEdit(owner *Form) *Edit {
	e := &Edit{}
	e.init(owner, backend.Edit)
	return e
}

// NewButton создаёт кнопку с владельцем, но без визуального Parent.
func NewButton(owner *Form) *Button {
	b := &Button{}
	b.init(owner, backend.Button)
	return b
}

// NewLabel создаёт метку с владельцем, но без визуального Parent.
func NewLabel(owner *Form) *Label {
	l := &Label{}
	l.init(owner, backend.Label)
	return l
}
func (c *component) init(owner *Form, kind backend.Kind) {
	if owner == nil {
		panic("ui: nil owner")
	}
	owner.guardAlive()
	if runActive.Load() {
		panic("ui: creation during Run is unsupported")
	}
	nextComponentOrder++
	c.order = nextComponentOrder
	c.owner, c.kind = owner, kind
	owner.owned[c] = struct{}{}
}
func (c *component) guardAlive() {
	if c.destroyed {
		panic("ui: operation on destroyed component")
	}
}

// Title возвращает заголовок формы.
func (f *Form) Title() string { f.guardAlive(); return f.text }

// SetTitle задаёт заголовок формы.
func (f *Form) SetTitle(title string) { f.guardAlive(); f.setText(title) }

// Owner возвращает неизменяемого владельца контрола.
func (c *control) Owner() *Form { c.guardAlive(); return c.owner }

// Parent возвращает визуальную форму-контейнер или nil.
func (c *control) Parent() *Form { c.guardAlive(); return c.parent }

// Text возвращает подготовленный текст контрола.
func (c *control) Text() string { c.guardAlive(); return c.text }

// SetText задаёт текст контрола.
func (c *control) SetText(text string) { c.guardAlive(); c.setText(text) }

// SetParent меняет только визуального родителя. MVP поддерживает настройку
// статического дерева до Run; nil отсоединяет контрол, сохраняя его состояние.
// Отказ не меняет ни одной связи или свойства.
func (c *control) SetParent(parent *Form) error {
	c.guardAlive()
	if parent != nil && parent.destroyed {
		return errors.New("ui: parent is destroyed")
	}
	if runActive.Load() {
		return errors.New("ui: parent changes during Run are unsupported")
	}
	if c.parent == parent {
		return nil
	}
	if c.parent != nil {
		delete(c.parent.children, &c.component)
	}
	c.parent = parent
	if parent == nil {
		c.cancelTextNotification()
	}
	if parent != nil {
		parent.children[&c.component] = struct{}{}
	}
	return nil
}

// Destroyed безопасен после Destroy и возвращает логическое состояние.
func (c *component) Destroyed() bool { return c.destroyed }

// Destroy немедленно уничтожает компонент, его owned-компоненты и визуальных
// потомков. Повторный вызов безопасен, включая пересечения двух деревьев.
func (c *component) Destroy() {
	if c.destroyed {
		return
	}
	c.destroyed = true
	c.cancelTextNotification()
	if r, ok := c.runtime.(interface{ componentDestroyed(*component) }); ok {
		r.componentDestroyed(c)
	}
	if c.owner != nil {
		delete(c.owner.owned, c)
	}
	if c.parent != nil {
		delete(c.parent.children, c)
	}
	c.owner, c.parent = nil, nil
	if c.form != nil {
		for child := range c.form.owned {
			child.Destroy()
		}
		for child := range c.form.children {
			child.Destroy()
		}
	}
}
