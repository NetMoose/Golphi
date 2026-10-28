package ui

// eventRuntime is implemented by the Run controller. enqueueEvent must schedule
// a later safe loop turn, never call inline. eventsRunning becomes false before
// stop/failure cleanup. syncText writes native text and returns the effective
// read-back value (normalization included); errors use runtime's panic boundary.
// The runtime installs this adapter only after native peers are initialized.
type eventRuntime interface {
	eventsRunning() bool
	enqueueEvent(func())
	syncText(*component, string) string
}

type eventState struct {
	runtime             eventRuntime
	onClick, onChange   func()
	writing, delivering bool
	base                string
	baseValid, pending  bool
	generation          uint64
}

// SetOnClick заменяет единственный обработчик кнопки; nil отключает событие.
func (b *Button) SetOnClick(handler func()) { b.guardAlive(); b.onClick = handler }

// SetOnChange заменяет обработчик реального изменения текста; nil отменяет
// также ещё не начатые и накопленные уведомления.
func (e *Edit) SetOnChange(handler func()) {
	e.guardAlive()
	e.onChange = handler
	if handler == nil {
		e.cancelTextNotification()
	}
}

func (c *component) eventsEnabled() bool {
	return !c.destroyed && c.runtime != nil && c.runtime.eventsRunning() &&
		(c.form != nil || (c.parent != nil && !c.parent.destroyed))
}
func (c *component) setText(text string) {
	old := c.text
	if old == text {
		return
	}
	if c.runtime != nil {
		// Synchronous native Change is folded into this transaction. Read-back is
		// authoritative, including when a backend normalizes to the previous value.
		c.writing = true
		func() { defer func() { c.writing = false }(); text = c.runtime.syncText(c, text) }()
	}
	c.applyText(old, text)
}

// nativeText accepts copied native input on the UI thread. Runtime routes Change
// by peer and must suppress callbacks before readiness and after stop/failure.
func (c *component) nativeText(text string) {
	if c.destroyed || c.writing {
		return
	}
	c.applyText(c.text, text)
}
func (c *component) applyText(old, text string) {
	c.text = text
	if old == text {
		return
	}
	if !c.eventsEnabled() || c.onChange == nil {
		c.cancelTextNotification()
		return
	}
	if c.delivering || c.pending {
		if !c.baseValid {
			c.base = old
			c.baseValid = true
		}
		return
	}
	c.deliverText()
}
func (c *component) deliverText() {
	if !c.eventsEnabled() || c.onChange == nil {
		c.cancelTextNotification()
		return
	}
	c.base, c.baseValid = c.text, true
	c.delivering = true
	defer func() { c.delivering = false }()
	c.onChange()
	if !c.eventsEnabled() || c.onChange == nil {
		c.cancelTextNotification()
		return
	}
	if !c.baseValid || c.text == c.base {
		c.baseValid = false
		return
	}
	c.pending = true
	generation := c.generation
	c.runtime.enqueueEvent(func() {
		if !c.pending || c.generation != generation {
			return
		}
		c.pending = false
		if !c.baseValid || c.text == c.base {
			c.baseValid = false
			return
		}
		c.deliverText()
	})
}
func (c *component) cancelTextNotification() {
	c.generation++
	c.pending, c.baseValid = false, false
}

// nativeClick is called only for backend Click events; admission is checked
// again so callbacks that were queued before logical destruction are inert.
func (c *component) nativeClick() {
	if c.eventsEnabled() && c.onClick != nil {
		c.onClick()
	}
}
