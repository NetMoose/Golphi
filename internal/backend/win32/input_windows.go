//go:build windows && amd64

package win32

import (
	"errors"
	"fmt"
	"github.com/NetMoose/Golphi/internal/backend"
	"sync"
	"unsafe"
)

var editTrampoline struct {
	sync.Once
	address uintptr
}

// The subclass keeps the direction that EM_GETSEL itself discards. Its process
// trampoline has no per-peer Go closure; peer records live through safe release.
func editProc(hwnd uintptr, msg uint32, w, l uintptr) (result uintptr) {
	registry.RLock()
	d := registry.owner
	registry.RUnlock()
	if d == nil {
		return 0
	}
	d.callbackDepth++
	defer func() { d.callbackDepth-- }()
	failure := backend.Capture(func() {
		var id backend.Peer
		var p peer
		for key, candidate := range d.peers {
			if candidate.hwnd == hwnd {
				id, p = key, candidate
				break
			}
		}
		if id == 0 {
			return
		}
		var previousLow, previousHigh uint32
		if msg == 0xb1 {
			previousLow, previousHigh = d.selection(hwnd)
		}
		// A prior caret observation cannot establish direction after user input,
		// text replacement, focus acquisition, or programmatic selection changes.
		if msg == 0xc || msg == 0xc2 || msg == 0xb1 || msg == 0x7 || msg == 0x100 || msg == 0x101 || msg == 0x102 || msg == 0x200 || msg == 0x201 || msg == 0x202 || msg == 0x203 {
			p.selectionObservation = caretObservation{}
			d.peers[id] = p
		}
		if msg == 0x8 { // WM_KILLFOCUS: caret still belongs to this edit before default handling.
			_ = d.rememberSelection(id, true)
		}
		result, _, _ = d.api["CallWindowProcW"].Call(p.oldProc, hwnd, uintptr(msg), w, l)
		if msg == 0xb1 { // EM_SETSEL: unchanged ordered range may leave active end unchanged.
			lo, hi := d.selection(hwnd)
			p = d.peers[id]
			changed := previousLow != lo || previousHigh != hi
			if changed || lo == hi {
				p.selectionAnchor = lo
				p.selectionCaret = hi
				if int32(w) > int32(l) && int32(l) >= 0 {
					p.selectionAnchor = hi
					p.selectionCaret = lo
				}
				p.selectionKnown = true
			}
			focus, _, _ := d.api["GetFocus"].Call()
			// Only a changed selection can supply a new explicit observation; a no-op
			// must not label an unchanged native caret with the requested direction.
			if focus == hwnd && changed {
				var cp point
				if ok, _, _ := d.api["GetCaretPos"].Call(uintptr(unsafe.Pointer(&cp))); ok != 0 {
					p.selectionObservation = caretObservation{X: cp.x, Y: cp.y, Valid: true}
				}
			}
			d.peers[id] = p
			if focus == hwnd {
				_ = d.rememberSelection(id, true)
			}
		} else if msg == 0x100 || msg == 0x101 || msg == 0x102 || msg == 0x200 || msg == 0x201 || msg == 0x202 || msg == 0x203 {
			focus, _, _ := d.api["GetFocus"].Call()
			if focus == hwnd {
				_ = d.rememberSelection(id, true)
			}
		}
	})
	if failure != nil {
		d.captureFailure(failure)
		return 0
	}
	return result
}
func (d *driver) selection(hwnd uintptr) (lo, hi uint32) {
	d.api["SendMessageW"].Call(hwnd, 0xb0, uintptr(unsafe.Pointer(&lo)), uintptr(unsafe.Pointer(&hi)))
	return
}
func (d *driver) rememberSelection(id backend.Peer, focused bool) error {
	p := d.peers[id]
	lo, hi := d.selection(p.hwnd)
	if lo == hi {
		p.selectionAnchor = lo
		p.selectionCaret = hi
		p.selectionKnown = true
		d.peers[id] = p
		return nil
	}
	if focused {
		knownBefore := p.selectionKnown
		p.selectionKnown = false
		d.peers[id] = p
		var caret point
		if _, err := d.call("GetCaretPos", uintptr(unsafe.Pointer(&caret))); err != nil {
			return err
		}
		observe := func(offset uint32) (caretObservation, uint32) {
			pos, _, _ := d.api["SendMessageW"].Call(p.hwnd, 0xd6, uintptr(offset), 0)
			if int32(pos) == -1 {
				return caretObservation{}, uint32(pos)
			}
			return caretObservation{X: int32(int16(pos)), Y: int32(int16(pos >> 16)), Valid: true}, uint32(pos)
		}
		cache := p.selectionObservation
		if !((p.selectionAnchor == lo && p.selectionCaret == hi) || (p.selectionAnchor == hi && p.selectionCaret == lo)) {
			cache = caretObservation{}
		}
		low, lowRaw := observe(lo)
		high, highRaw := observe(hi)
		reverse, err := singleLineSelectionDirection(caretObservation{X: caret.x, Y: caret.y, Valid: true}, low, high, cache, p.selectionCaret == lo)
		if err != nil {
			return fmt.Errorf("native selection hwnd=%#x focused=%t utf16=[%d,%d] cached=[%d,%d] knownBefore=%t rawPositions=[%#x,%#x]: %w", p.hwnd, focused, lo, hi, p.selectionAnchor, p.selectionCaret, knownBefore, lowRaw, highRaw, err)
		}
		p.selectionAnchor = lo
		p.selectionCaret = hi
		if reverse {
			p.selectionAnchor = hi
			p.selectionCaret = lo
		}
		p.selectionKnown = true
		d.peers[id] = p
		return nil
	}
	if p.selectionKnown && ((p.selectionAnchor == lo && p.selectionCaret == hi) || (p.selectionAnchor == hi && p.selectionCaret == lo)) {
		return nil
	}
	return errors.New("win32: unfocused selection direction unavailable")
}
func (d *driver) SnapshotInput(id backend.Peer) (backend.InputState, error) {
	p, err := d.get(id)
	if err != nil {
		return backend.InputState{}, err
	}
	if p.kind != backend.Edit {
		return backend.InputState{}, errors.New("win32: input requires Edit")
	}
	text, err := d.Text(id)
	if err != nil {
		return backend.InputState{}, err
	}
	focus, _, _ := d.api["GetFocus"].Call()
	if err = d.rememberSelection(id, focus == p.hwnd); err != nil {
		return backend.InputState{}, fmt.Errorf("SnapshotInput peer=%d: %w", id, err)
	}
	p = d.peers[id]
	a, err := utf16ToScalar(text, int(p.selectionAnchor))
	if err != nil {
		return backend.InputState{}, err
	}
	c, err := utf16ToScalar(text, int(p.selectionCaret))
	if err != nil {
		return backend.InputState{}, err
	}
	return backend.InputState{Text: text, Anchor: a, Caret: c, Focused: focus == p.hwnd}, nil
}
func (d *driver) restoreInput(id backend.Peer, s backend.InputState) error {
	p, err := d.get(id)
	if err != nil {
		return err
	}
	actual, err := d.Text(id)
	if err != nil {
		return err
	}
	if actual != s.Text {
		return fmt.Errorf("win32: effective text differs during input restore")
	}
	a, err := scalarToUTF16(actual, s.Anchor)
	if err != nil {
		return err
	}
	c, err := scalarToUTF16(actual, s.Caret)
	if err != nil {
		return err
	}
	// Force a real ordered-range transition before setting its active end.
	// The Windows log showed a same-range reverse EM_SETSEL leaving caret at high.
	// No text changes, application notifications, dispatch, or timing wait.
	if a != c {
		d.api["SendMessageW"].Call(p.hwnd, 0xb1, uintptr(a), uintptr(a))
	}
	d.api["SendMessageW"].Call(p.hwnd, 0xb1, uintptr(a), uintptr(c))
	lo, hi := d.selection(p.hwnd)
	min, max := a, c
	if min > max {
		min, max = max, min
	}
	if lo != uint32(min) || hi != uint32(max) {
		return errors.New("win32: selection restore readback mismatch")
	}
	focus, _, _ := d.api["GetFocus"].Call()
	if focus == p.hwnd {
		actual, err := d.SnapshotInput(id)
		if err != nil {
			return fmt.Errorf("selection restore direction readback: %w", err)
		}
		if actual.Anchor != s.Anchor || actual.Caret != s.Caret {
			return fmt.Errorf("selection restore direction mismatch: requested scalar=[%d,%d] observed=[%d,%d]", s.Anchor, s.Caret, actual.Anchor, actual.Caret)
		}
	}
	return nil
}
