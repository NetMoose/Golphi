//go:build windows && amd64

package win32

import (
	"errors"
	"unsafe"

	"github.com/NetMoose/Golphi/internal/backend"
)

const (
	spiGetNonClientMetrics = 0x0029
	wmSetFont              = 0x0030
)

type logFont struct {
	height, width, escapement, orientation, weight int32
	italic, underline, strikeOut, charSet          byte
	outPrecision, clipPrecision, quality, pitch    byte
	faceName                                       [32]uint16
}

type nonClientMetrics struct {
	size                                                                uint32
	borderWidth, scrollWidth, scrollHeight, captionWidth, captionHeight int32
	captionFont                                                         logFont
	smallCaptionWidth, smallCaptionHeight                               int32
	smallCaptionFont                                                    logFont
	menuWidth, menuHeight                                               int32
	menuFont, statusFont, messageFont                                   logFont
	paddedBorderWidth                                                   int32
}

func (d *driver) createSystemFont() (uintptr, error) {
	metrics := nonClientMetrics{size: uint32(unsafe.Sizeof(nonClientMetrics{}))}
	const info = "SystemParametersInfoForDpi"
	r, _, nativeErr := d.api[info].Call(spiGetNonClientMetrics, uintptr(metrics.size), uintptr(unsafe.Pointer(&metrics)), 0, uintptr(d.dpi))
	if _, err := checkedCall(info, r, nativeErr); err != nil {
		return 0, err
	}
	// lfMessageFont already describes the requested DPI.
	const create = "CreateFontIndirectW"
	r, _, nativeErr = d.api[create].Call(uintptr(unsafe.Pointer(&metrics.messageFont)))
	return checkedCall(create, r, nativeErr)
}

func usesSystemFont(kind backend.Kind) bool {
	return kind == backend.Button || kind == backend.Edit || kind == backend.Label
}

func (d *driver) deleteSystemFont() error {
	for _, p := range d.peers {
		if usesSystemFont(p.kind) {
			return errors.New("win32: system font retained for surviving native controls")
		}
	}
	// Native controls manage their drawing DCs. The backend does not select
	// this font into a DC, and the message loop has returned before Close.
	_, err := d.call("DeleteObject", d.font)
	if err == nil {
		d.font = 0
	}
	return err
}
