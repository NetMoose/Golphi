package ui

import (
	"errors"
	"reflect"

	"github.com/NetMoose/Golphi/internal/backend"
	"github.com/NetMoose/Golphi/internal/uithread"
)

// Form — логическая форма и владелец контролов экспериментального MVP.
// Создание формы не обращается к оконной системе.
// Component is an experimental sealed role; third-party implementations are unsupported.
type Component interface {
	Owner() Component
	Parent() Container
	Destroy()
	Destroyed() bool
	componentIdentity() *component
}

// Container is the experimental visual-parent role implemented by Form and Panel.
type Container interface {
	Component
	containerRole()
}

type Form struct{ component }

// Panel is a logical visual container without text or caption properties.
// Native rendering depends on backend support.
type Panel struct{ visual }

func (*Form) containerRole()  {}
func (*Panel) containerRole() {}

// Edit — однострочное поле ввода.
type Edit struct{ control }

// Button — кнопка.
type Button struct{ control }

// Label — текстовая метка.
type Label struct{ control }

var nextComponentOrder uint64

type component struct {
	order    uint64
	kind     backend.Kind
	owner    Component
	parent   Container
	owned    map[*component]struct{}
	children map[*component]struct{}
	text     string
	// savedInput belongs to logical preparation/detachment, never live peers.
	savedInput *backend.InputState
	eventState
	bounds backend.Bounds
	// started is latched by Run before native initialization for every participant.
	started   bool
	destroyed bool
	// form is set only for forms; graph edges refer to component identities.
	form *Form
}

type visual struct{ component }
type control struct{ visual }

func (c *component) componentIdentity() *component { return c }

// NewForm создаёт логическую форму без native-ресурсов.
func NewForm() *Form {
	guardPreparationThread()
	if runActive.Load() {
		panic("ui: creation during Run is unsupported")
	}
	f := &Form{}
	nextComponentOrder++
	f.order = nextComponentOrder
	f.kind, f.form = backend.Form, f
	return f
}

// NewEdit создаёт логическое поле с владельцем, но без визуального Parent.
// Во время Run конструкторы Edit/Button/Label/Panel требуют UI-поток и не
// создают native-ресурсов.
func NewEdit(owner Component) *Edit {
	e := &Edit{}
	e.init(owner, backend.Edit)
	return e
}

// NewButton создаёт кнопку с владельцем, но без визуального Parent.
func NewButton(owner Component) *Button {
	b := &Button{}
	b.init(owner, backend.Button)
	return b
}

// NewLabel создаёт метку с владельцем, но без визуального Parent.
func NewLabel(owner Component) *Label {
	l := &Label{}
	l.init(owner, backend.Label)
	return l
}

// NewPanel creates a logical container with an immutable owner and no Parent.
func NewPanel(owner Component) *Panel {
	p := &Panel{}
	p.init(owner, backend.Panel)
	return p
}

func (c *component) init(owner Component, kind backend.Kind) {
	if owner == nil || reflect.ValueOf(owner).IsNil() {
		panic("ui: nil owner")
	}
	guardPreparationThread()
	owner.componentIdentity().guardAlive()
	nextComponentOrder++
	c.order = nextComponentOrder
	c.owner, c.kind = owner, kind
	o := owner.componentIdentity()
	if o.owned == nil {
		o.owned = make(map[*component]struct{})
	}
	o.owned[c] = struct{}{}
	if root := c.ownerRoot(); root != nil {
		if r, ok := root.runtime.(*application); ok {
			r.participants = append(r.participants, c)
			c.runtime = r
		}
	}
}

// ownerRoot follows immutable ownership, independently of visual attachment.
func (c *component) ownerRoot() *Form {
	for c.owner != nil {
		c = c.owner.componentIdentity()
	}
	return c.form
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

// Owner возвращает неизменяемого владельца или nil для корневой Form.
func (c *component) Owner() Component { c.guardAlive(); return c.owner }

// Parent возвращает визуальный контейнер или nil.
func (c *component) Parent() Container { c.guardAlive(); return c.parent }

// Text возвращает подготовленный текст контрола.
func (c *control) Text() string { c.guardAlive(); return c.text }

// SetText задаёт текст контрола.
func (c *control) SetText(text string) { c.guardAlive(); c.setText(text) }

// SetParent changes the visual parent synchronously. During Run it requires
// the UI thread and backend transaction support; the same Parent is a no-op.
// nil отсоединяет контрол, сохраняя его состояние.
// An ordinary failure preserves links and properties. Failed native restoration
// stops the runtime and is also reported by Run.
func (c *visual) SetParent(parent Container) error {
	c.guardAlive()
	guardPreparationThread()
	if parent != nil && reflect.ValueOf(parent).IsNil() {
		parent = nil
	}
	if parent != nil && parent.componentIdentity().destroyed {
		return errors.New("ui: parent is destroyed")
	}
	if parent != nil {
		p := parent.componentIdentity()
		if p == &c.component {
			return errors.New("ui: component cannot parent itself")
		}
		for ancestor := p; ancestor != nil; {
			if ancestor == &c.component {
				return errors.New("ui: parent cycle")
			}
			if ancestor.parent == nil {
				break
			}
			ancestor = ancestor.parent.componentIdentity()
		}
		if c.ownerRoot() != p.ownerRoot() {
			return errors.New("ui: parent belongs to a different form")
		}
	}
	if c.parent == parent {
		return nil
	}
	if runActive.Load() && (c.attached() || (parent != nil && parent.componentIdentity().attached())) {
		if r, ok := c.ownerRoot().runtime.(*application); ok {
			return r.setParent(&c.component, parent)
		}
		return errors.New("ui: form runtime is unavailable")
	}
	if c.parent != nil {
		delete(c.parent.componentIdentity().children, &c.component)
	}
	c.parent = parent
	if parent == nil {
		c.cancelTextNotification()
	}
	if parent != nil {
		p := parent.componentIdentity()
		if p.children == nil {
			p.children = make(map[*component]struct{})
		}
		p.children[&c.component] = struct{}{}
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
		delete(c.owner.componentIdentity().owned, c)
	}
	if c.parent != nil {
		delete(c.parent.componentIdentity().children, c)
	}
	c.owner, c.parent = nil, nil
	for child := range c.owned {
		child.Destroy()
	}
	for child := range c.children {
		child.Destroy()
	}
}

// This guard is deliberately scoped to logical constructors and preparation
// setters. Other APIs retain their existing threading contract.
func guardPreparationThread() {
	if runActive.Load() && !uithread.IsUI() {
		panic("ui: operation requires the UI thread during Run")
	}
}

// attached follows the entire visual chain, independently of immutable Owner.
func (c *component) attached() bool {
	for c != nil {
		if c.form != nil {
			return true
		}
		if c.parent == nil {
			return false
		}
		c = c.parent.componentIdentity()
	}
	return false
}
