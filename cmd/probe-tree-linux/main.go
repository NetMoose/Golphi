//go:build linux

// Native characterization experiment; deliberately independent of the runtime Driver.
package main

import (
	"fmt"
	"github.com/NetMoose/Golphi/internal/uithread"
	"github.com/ebitengine/purego"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if !uithread.IsUI() {
		return fmt.Errorf("not startup thread")
	}
	lib, err := purego.Dlopen("libgtk-4.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return err
	}
	// Never dlclose GTK: its process-global handlers/TLS outlive our windows.
	bind := func(fn any, name string) { purego.RegisterLibFunc(fn, lib, name) }
	var init func() int32
	bind(&init, "gtk_init_check")
	if init() == 0 {
		return fmt.Errorf("GTK init failed DISPLAY=%q", os.Getenv("DISPLAY"))
	}
	var display func() uintptr
	bind(&display, "gdk_display_get_default")
	var xtype func() uintptr
	bind(&xtype, "gdk_x11_display_get_type")
	var isa func(uintptr, uintptr) int32
	bind(&isa, "g_type_check_instance_is_a")
	if isa(display(), xtype()) == 0 {
		return fmt.Errorf("requires X11")
	}
	var winNew, fixedNew, scrollNew, entryNew func() uintptr
	bind(&winNew, "gtk_window_new")
	bind(&fixedNew, "gtk_fixed_new")
	bind(&scrollNew, "gtk_scrolled_window_new")
	bind(&entryNew, "gtk_entry_new")
	var setChild, scrollChild func(uintptr, uintptr)
	bind(&setChild, "gtk_window_set_child")
	bind(&scrollChild, "gtk_scrolled_window_set_child")
	var size func(uintptr, int32, int32)
	bind(&size, "gtk_widget_set_size_request")
	var policy func(uintptr, int32, int32)
	bind(&policy, "gtk_scrolled_window_set_policy")
	var put func(uintptr, uintptr, float64, float64)
	bind(&put, "gtk_fixed_put")
	var remove func(uintptr, uintptr)
	bind(&remove, "gtk_fixed_remove")
	var sink func(uintptr) uintptr
	bind(&sink, "g_object_ref_sink")
	var unref, present, destroy func(uintptr)
	bind(&unref, "g_object_unref")
	bind(&present, "gtk_window_present")
	bind(&destroy, "gtk_window_destroy")
	var width, height func(uintptr) int32
	bind(&width, "gtk_widget_get_width")
	bind(&height, "gtk_widget_get_height")
	var iterate func(uintptr, int32) int32
	bind(&iterate, "g_main_context_iteration")
	pump := func() {
		deadline := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(deadline) {
			for time.Now().Before(deadline) && iterate(0, 0) != 0 {
			}
			time.Sleep(time.Millisecond)
		}
	}
	var pick func(uintptr, float64, float64, uint32) uintptr
	bind(&pick, "gtk_widget_pick")
	var parent func(uintptr) uintptr
	bind(&parent, "gtk_widget_get_parent")
	var focus func(uintptr) int32
	bind(&focus, "gtk_widget_grab_focus")
	var windowFocus func(uintptr) uintptr
	bind(&windowFocus, "gtk_window_get_focus")
	var setText func(uintptr, string)
	bind(&setText, "gtk_editable_set_text")
	var getText func(uintptr) string
	bind(&getText, "gtk_editable_get_text")
	var selectRegion func(uintptr, int32, int32)
	bind(&selectRegion, "gtk_editable_select_region")
	var position func(uintptr) int32
	bind(&position, "gtk_editable_get_position")
	var bounds func(uintptr, *int32, *int32) int32
	bind(&bounds, "gtk_editable_get_selection_bounds")
	var connect func(uintptr, string, uintptr, uintptr, uintptr, uint32) uint64
	bind(&connect, "g_signal_connect_data")
	var disconnect func(uintptr, uint64)
	bind(&disconnect, "g_signal_handler_disconnect")
	var weak func(uintptr, uintptr, uintptr)
	bind(&weak, "g_object_weak_ref")
	finalized, callbacks := 0, 0
	callbackThread := uint64(0)
	var callbackPanic any
	changed := purego.NewCallback(func(_, _ uintptr) {
		defer func() {
			if p := recover(); p != nil {
				callbackPanic = p
			}
		}()
		callbacks++
		callbackThread = uithread.Current()
	})
	gone := purego.NewCallback(func(_, _ uintptr) { finalized++ })
	win := winNew()
	defer destroy(win)
	root := fixedNew()
	setChild(win, root)
	panel := func(w, h int32) (uintptr, uintptr) {
		s, f := scrollNew(), fixedNew()
		policy(s, 3, 3)
		size(s, w, h)
		scrollChild(s, f)
		var child func(uintptr) uintptr
		bind(&child, "gtk_scrolled_window_get_child")
		var autoFocus func(uintptr, int32)
		bind(&autoFocus, "gtk_viewport_set_scroll_to_focus")
		autoFocus(child(s), 0)
		return s, f
	}
	outer, content := panel(260, 180)
	put(root, outer, 10, 10)
	inner, a := panel(140, 90)
	put(content, inner, 10, 10)
	var cssNew func() uintptr
	bind(&cssNew, "gtk_css_provider_new")
	var cssLoad func(uintptr, string, int64)
	bind(&cssLoad, "gtk_css_provider_load_from_data")
	var cssAdd func(uintptr, uintptr, uint32)
	bind(&cssAdd, "gtk_style_context_add_provider_for_display")
	css := cssNew()
	cssLoad(css, "scrolledwindow {background-color: #ffffff;}", -1)
	cssAdd(display(), css, 600)
	unref(css)
	present(win)
	pump()
	ow, oh, iw, ih := width(outer), height(outer), width(inner), height(inner)
	beforePixels, err := renderPixels(lib, win, outer, ow, oh, "/tmp/golphi-tree-linux-before.png")
	if err != nil {
		return err
	}
	// A huge child must not influence either panel's allocation.
	var drawingNew func() uintptr
	bind(&drawingNew, "gtk_drawing_area_new")
	var setDraw func(uintptr, uintptr, uintptr, uintptr)
	bind(&setDraw, "gtk_drawing_area_set_draw_func")
	var rgb func(uintptr, float64, float64, float64)
	bind(&rgb, "cairo_set_source_rgb")
	var paint func(uintptr)
	bind(&paint, "cairo_paint")
	draw := purego.NewCallback(func(_ uintptr, cr uintptr, _ int32, _ int32, _ uintptr) { rgb(cr, 1, 0, 1); paint(cr) })
	huge := drawingNew()
	setDraw(huge, draw, 0, 0)
	size(huge, 1200, 600)
	put(a, huge, 100, 60)
	pump()
	noGrowth := ow == 260 && oh == 180 && iw == 140 && ih == 90 && width(outer) == ow && height(outer) == oh && width(inner) == iw && height(inner) == ih
	// Hit outside inner's right edge, but inside outer. It must not reach huge.
	hit := pick(outer, 155, 80, 0)
	clipped := true
	for p := hit; p != 0; p = parent(p) {
		if p == huge {
			clipped = false
		}
	}
	inside := false
	for p := pick(inner, 110, 70, 0); p != 0; p = parent(p) {
		if p == huge {
			inside = true
		}
	}
	clipped = clipped && inside
	afterPixels, err := renderPixels(lib, win, outer, ow, oh, "/tmp/golphi-tree-linux-after.png")
	if err != nil {
		return err
	}
	insideChanged, outsideChanged := 0, 0
	for y := int32(0); y < oh; y++ {
		for x := int32(0); x < ow; x++ {
			i := int((y*ow + x) * 4)
			different := false
			for c := 0; c < 4; c++ {
				if beforePixels[i+c] != afterPixels[i+c] {
					different = true
				}
			}
			if different {
				if x >= 10 && x < 150 && y >= 10 && y < 100 {
					insideChanged++
				} else {
					outsideChanged++
				}
			}
		}
	}
	pixelsClipped := insideChanged > 0 && outsideChanged == 0
	fmt.Printf("clip_pixels=%t inside_changed=%d outside_changed=%d snapshots=/tmp/golphi-tree-linux-{before,after}.png\n", pixelsClipped, insideChanged, outsideChanged)
	if !pixelsClipped {
		return fmt.Errorf("render clipping failed")
	}
	fmt.Printf("outer=%dx%d inner=%dx%d huge=%dx%d no_growth=%t clip_pick=%t\n", ow, oh, iw, ih, width(huge), height(huge), noGrowth, clipped)
	if !noGrowth || !clipped {
		return fmt.Errorf("panel geometry/clipping failed")
	}
	other, b := panel(140, 90)
	put(content, other, 160, 10)
	pump()
	var hadj, vadj func(uintptr) uintptr
	bind(&hadj, "gtk_scrolled_window_get_hadjustment")
	bind(&vadj, "gtk_scrolled_window_get_vadjustment")
	var adjValue func(uintptr) float64
	bind(&adjValue, "gtk_adjustment_get_value")
	adjustments := func() []float64 {
		values := []float64{}
		for _, p := range []uintptr{outer, inner, other} {
			values = append(values, adjValue(hadj(p)), adjValue(vadj(p)))
		}
		return values
	}
	var childPosition func(uintptr, uintptr, *float64, *float64)
	bind(&childPosition, "gtk_fixed_get_child_position")
	stableCoordinates := func() bool {
		var x, y float64
		childPosition(content, other, &x, &y)
		if x != 160 || y != 10 {
			return false
		}
		childPosition(a, huge, &x, &y)
		return x == 100 && y == 60
	}
	initialAdjustments := adjustments()
	stableAdjustments := func() bool {
		for i, v := range adjustments() {
			if v != initialAdjustments[i] {
				return false
			}
		}
		return true
	}
	entry := entryNew()
	sink(entry)
	put(a, entry, 0, 0)
	weak(entry, gone, 0)
	handler := connect(entry, "changed", changed, 0, 0, 0)
	text := "А😀Б𐐷В" // 5 Unicode scalar values, 14 UTF-8 bytes.
	setText(entry, text)
	focus(entry)
	pump()
	selectRegion(entry, 4, 1)
	pump()
	var lo, hi int32
	bounds(entry, &lo, &hi)
	caret := position(entry)
	descendant := func(p, ancestor uintptr) bool {
		for p != 0 {
			if p == ancestor {
				return true
			}
			p = parent(p)
		}
		return false
	}
	check := func(e uintptr) bool {
		var l, h int32
		ok := bounds(e, &l, &h) != 0
		return ok && l == 1 && h == 4 && position(e) == caret && descendant(windowFocus(win), e)
	}
	if lo != 1 || hi != 4 || !check(entry) {
		return fmt.Errorf("initial Unicode selection/focus invalid: %d:%d caret=%d", lo, hi, caret)
	}
	anchor := lo
	if caret == lo {
		anchor = hi
	}
	remove(a, entry)
	put(b, entry, 0, 0)
	focus(entry)
	pump()
	selectRegion(entry, anchor, caret)
	pump()
	moved := check(entry) && stableAdjustments() && stableCoordinates()
	var ml, mh int32
	bounds(entry, &ml, &mh)
	fmt.Printf("move bounds=%d:%d caret=%d focused=%t\n", ml, mh, position(entry), descendant(windowFocus(win), entry))
	saved := getText(entry)
	disconnect(entry, handler)
	before := callbacks
	setText(entry, "disconnect verification")
	disconnected := callbacks == before
	remove(b, entry)
	unref(entry)
	replacement := entryNew()
	sink(replacement)
	weak(replacement, gone, 0)
	put(b, replacement, 0, 0)
	setText(replacement, saved)
	focus(replacement)
	pump()
	selectRegion(replacement, anchor, caret)
	pump()
	readBack := getText(replacement)
	recreated := readBack == saved && saved == text && check(replacement) && stableAdjustments() && stableCoordinates()
	fmt.Printf("adjustments_initial=%v adjustments_final=%v no_auto_scroll=%t fixed_coordinates=%t replacement_text=%q\n", initialAdjustments, adjustments(), stableAdjustments(), stableCoordinates(), readBack)
	remove(b, replacement)
	unref(replacement)
	pump()
	same := callbackThread == uithread.Startup() && callbackPanic == nil && callbacks > 0
	fmt.Printf("selection=%d:%d caret=%d text=%q move_state=%t recreate_state=%t callbacks=%d callback_same_thread=%t disconnect=%t finalized=%d\n", lo, hi, caret, saved, moved, recreated, callbacks, same, disconnected, finalized)
	if !moved || !recreated || !same || !disconnected || finalized != 2 {
		return fmt.Errorf("state/lifecycle assertion failed")
	}
	return nil
}
