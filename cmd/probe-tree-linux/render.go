//go:build linux

package main

import (
	"fmt"
	"github.com/ebitengine/purego"
	"image"
	"image/color"
	"image/png"
	"os"
)

// Native GTK snapshot -> GSK renderer -> downloaded pixels; no compositor screenshot.
func renderPixels(lib, win, widget uintptr, w, h int32, path string) ([]byte, error) {
	bind := func(fn any, name string) { purego.RegisterLibFunc(fn, lib, name) }
	var paintableNew func(uintptr) uintptr
	bind(&paintableNew, "gtk_widget_paintable_new")
	var snapshotNew func() uintptr
	bind(&snapshotNew, "gtk_snapshot_new")
	var snapshot func(uintptr, uintptr, float64, float64)
	bind(&snapshot, "gdk_paintable_snapshot")
	var nodeFromSnapshot func(uintptr) uintptr
	bind(&nodeFromSnapshot, "gtk_snapshot_free_to_node")
	var renderer func(uintptr) uintptr
	bind(&renderer, "gtk_native_get_renderer")
	var render func(uintptr, uintptr, *[4]float32) uintptr
	bind(&render, "gsk_renderer_render_texture")
	var download func(uintptr, []byte, uintptr)
	bind(&download, "gdk_texture_download")
	var unref, nodeUnref func(uintptr)
	bind(&unref, "g_object_unref")
	bind(&nodeUnref, "gsk_render_node_unref")
	p := paintableNew(widget)
	defer unref(p)
	s := snapshotNew()
	snapshot(p, s, float64(w), float64(h))
	node := nodeFromSnapshot(s)
	if node == 0 {
		return nil, fmt.Errorf("empty render node")
	}
	defer nodeUnref(node)
	rect := [4]float32{0, 0, float32(w), float32(h)}
	texture := render(renderer(win), node, &rect)
	if texture == 0 {
		return nil, fmt.Errorf("render_texture failed")
	}
	defer unref(texture)
	pixels := make([]byte, int(w*h*4))
	download(texture, pixels, uintptr(w*4))
	// gdk_texture_download uses Cairo ARGB32; on Linux amd64 bytes are BGRA.
	img := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < len(pixels); i += 4 {
		img.SetNRGBA((i/4)%int(w), (i/4)/int(w), color.NRGBA{pixels[i+2], pixels[i+1], pixels[i], pixels[i+3]})
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return nil, err
	}
	return pixels, nil
}
