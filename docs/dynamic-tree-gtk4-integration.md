# GTK4 integration of the dynamic tree

The production GTK driver now implements Panel, prepared startup input,
SnapshotInput and the Prepare/Commit/Rollback/Cleanup/Finish protocol. The
checks below concern Linux amd64, GTK 4.14.5 (Ubuntu package
4.14.5+ds-0ubuntu0.10), X11 DISPLAY=:0. GDK_SCALE/GDK_DPI_SCALE were unset;
this is not a scale matrix or a Wayland acceptance result.

Panel uses GtkScrolledWindow with external policies, an internal GtkFixed and
viewport scroll-to-focus disabled. Child size does not propagate to the panel;
nested viewports provide drawing and hit clipping. Native minimum sizes remain
the accepted MVP sizing policy.

Transactions fence native delivery before snapshots. Moves snapshot fresh Edit input and the window's exact focus widget.
An attached subtree containing Edit is recreated in full while the old subtree
remains realized. Focus and selection transfer to the new peers before the old
root is hidden/unparented. Subtrees without Edit retain their native objects and
reparent the root. Dynamic creation is initially hidden. Before
returning successful bindings, the driver shows the complete subtree and invokes
GtkFixed layout synchronously through gtk_widget_allocate on the form content,
using its current logical allocation (configured client size before allocation)
and the GtkWindow-assigned content origin read through gtk_widget_compute_bounds.
A GskTransform translation preserves that origin, including the CSD titlebar
offset; gtk_widget_allocate consumes the transform. This is specific to the
GtkWindow content translation, not a general arbitrary-transform guarantee.
It verifies nonzero allocations, restores the exact window focus target and
restores/checks selection. No production sleep or nested event dispatch is used.
A failed readiness check rolls back the parent, allocation, focus and input.
Rollback verifies the original subtree has usable allocation; failure latches
native delivery shutdown and reports a fatal restoration error. For replacement,
rollback restores focus/selection to the still-realized old peers before hiding
and unparenting the attempted replacement.

GTK character offsets represent Unicode scalar positions here. GtkEntry's
editable delegate is essential: gtk_editable_get_position on GtkEntry goes
through its delegating selection implementation and can return the sorted end
for reversed selection. Reading its GtkText delegate yields the actual caret.
Startup and attachment apply prepared positions without requesting focus.
Detachment returns an owned input snapshot with Focused=false; reattachment does
not restore focus. Moves preserve the focused logical Button or Edit even when peers are replaced.

After all fallible work completes, the driver reconnects signals with fresh
monotonic registry tokens and generations. Old tokens are removed, so stale
callbacks cannot acquire a new generation through a reused native handle.
GTK changed/clicked emissions are synchronous; technical signals during native
setters, moves and restore are suppressed under the fence. Late old-token
injection in tests is a separate check of the token gate, not proof of an
asynchronous toolkit event queue.

Retired subtrees and failed attempts remain driver-owned. Finish invalidates
routes but performs no physical object release. Run releases retirements at its
outer safe boundary, after native iteration/dispatch returned, or Close releases
them. Merely returning from the Go callback is not treated as leaving the native
signal emission. Native tests use g_object_weak_ref to observe actual object
finalization, not just registry removal. No RSS decrease is required.

## Verification, 2026-10-29

Commands (native command required local desktop access outside the restricted
socket sandbox):

```sh
CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off GOLPHI_GTK_NATIVE=1 DISPLAY=:0 GDK_BACKEND=x11 go test ./internal/backend/gtk4 -count=1 -v -timeout 30s
CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go test -count=1 ./...
CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off go vet ./internal/backend/gtk4
```

All passed at the initial checkpoint. Native TestNative, TestNativeTree and TestNativePanelClipping ran
alongside four ordinary GTK tests. Tree assertions cover all four transitions,
prepared reverse Unicode selection, synchronous mapped focus/allocation, a
focused Button move, no latent selection/focus change after later frame work,
a simulated native allocation-readiness failure and ordinary rollback, late
partial creation failure, quiet materialization, registry rejection, deferred
actual finalization during a real changed callback, and empty final registries.
The clipping test checks production nested Panel allocation and hit testing,
then renders its child snapshot through GSK: 1200 changed pixels inside and 0
outside. Rendering-only waits do not participate in transaction readiness.

The render helper uses gtk_widget_snapshot_child rather than the cached
GtkWidgetPaintable frame: the latter returned an empty node after explicit
allocation in this environment. This was a test observation, not a relaxation
of readiness or clipping assertions.

Official references were consulted after Context7 resolve failed with a
transport error:

- [Widget allocation](https://docs.gtk.org/gtk4/method.Widget.allocate.html)
- [Window focus](https://docs.gtk.org/gtk4/method.Window.set_focus.html)
- [Editable selection](https://docs.gtk.org/gtk4/method.Editable.select_region.html)
- [GTK 4.14.5 editable implementation](https://github.com/GNOME/gtk/blob/4.14.5/gtk/gtkeditable.c)
- [GTK 4.14.5 text implementation](https://github.com/GNOME/gtk/blob/4.14.5/gtk/gtktext.c)

This does not close IME, grapheme semantics, accessibility, multiple windows,
Wayland, other scales/GTK versions, cleanup retry policy, public diagnostics or
the remaining design open questions. The explicit container allocation path
is intentionally a focused review point: it relies on GTK's widget allocation
API and the current form content size, not on a new public sizing guarantee.
The public core-to-native integration is checked separately by its native suite.

A further regression injects a readiness readback failure after a new subtree
has been shown. Ordinary rollback hides/unparents that attempted root before
returning, while physical release remains deferred. The final native suite
passed again after this correction (all seven tests, 0.914s).


## Review corrections and activated-window acceptance

Review reproduced two defects: root content moved from `[0,46,640,434]` to
`[0,0,640,434]` with GTK_CSD=1, and a persistently suppressed native allocation
caused rollback to return nil with unusable geometry. The origin-preserving
allocation and rollback readback checks above correct these defects.

Completing initial X11 activation before establishing reverse selection also
exposed a PRIMARY clipboard race in the former Edit reparent path. GtkText
unrealize clears its PRIMARY content; rapid relinquish/reclaim can produce equal
X11 timestamps, allowing a later SelectionClear to collapse the restored
selection. Logging changed timing and could mask the failure. This is why the
final driver replaces Edit-containing subtrees while old widgets remain realized.
It does not restore selection asynchronously or write unrelated clipboard data.
A rollback that already transferred focus/PRIMARY transfers them back before
retiring the failed replacement.

Final ordinary native suite: all eight tests PASS, 1.846s. Additional focused
command (same native/cache prefix as above, adding GTK_CSD=1):

```sh
CGO_ENABLED=0 GOCACHE=/tmp/golphi-gocache GOMODCACHE=/tmp/golphi-gomodcache GOPROXY=off GOLPHI_GTK_NATIVE=1 DISPLAY=:0 GDK_BACKEND=x11 GTK_CSD=1 go test ./internal/backend/gtk4 -run 'TestNative(Tree|ReplacementSelection)$' -count=3 -v -timeout 30s
```

PASS, 3.589s. Each CSD run preserves `[0,46,640,434]` immediately and after later
frame work. Mixed replacement performs nine four-node subtree moves alternating
focused Edit, focused Button and external Button; all result IDs/generations and
fresh native text are checked. All 36 old peers finalize exactly once; a real
clicked signal on a retired Button before release and later technical events
remain silent. The test also covers ordinary rollback after replacement takes
focus/PRIMARY and fatal refusal after persistent allocation failure on the
retained-peer path. The full ordinary suite and GTK vet pass after the changes.

Additional source references:

- [Widget bounds](https://docs.gtk.org/gtk4/method.Widget.compute_bounds.html)
- [GSK translation](https://docs.gtk.org/gsk4/method.Transform.translate.html)
- [GTK 4.14.5 X11 clipboard](https://github.com/GNOME/gtk/blob/4.14.5/gdk/x11/gdkclipboard-x11.c)

This targeted CSD regression does not establish a new decoration, scale or
backend support matrix. The former direct-Edit-move test result is superseded by
the activated-window replacement acceptance above.
