// Package backend defines the internal native MVP boundary. It neither loads
// native libraries nor selects a platform implementation when imported.
package backend

// Peer is a backend-issued identity, not an HWND, GTK pointer, or Go pointer.
// Zero is invalid (and denotes no parent). IDs must not be reused within a run.
type Peer uint64

type Kind uint8

const (
	Form Kind = iota + 1
	Edit
	Button
	Label
	Panel
)

// Bounds are logical client coordinates; conversion is backend-owned.
type Bounds struct{ X, Y, Width, Height int }

type Config struct {
	// Input optionally supplies prepared Edit positions for Text. It does not
	// request focus. Static adapters defer consuming it until tree support.
	Input  *InputState
	Kind   Kind
	Parent Peer
	Text   string
	Bounds Bounds
}

type EventKind uint8

const (
	Click EventKind = iota + 1
	Change
	CloseRequest
)

// Event contains Go-owned data. Change.Text is a snapshot copied before returning
// from the native callback. CloseRequest asks the core to begin ordinary stop.
type Event struct {
	Kind EventKind
	Peer Peer
	Text string
	// Generation is a routing epoch for dynamic peers; zero is the legacy
	// static path. Native signals retain the epoch captured at their origin.
	Generation uint64
}

// Callbacks are retained from Init until Close. Failure is an internal,
// non-panicking hook that immediately stops queue admission and application
// delivery and records the panic. It must never invoke user code. Adapters
// latch failure before invoking it, suppress further events, and also return
// the first captured panic from Run.
type Callbacks struct {
	Event   func(Event)
	Failure func(*Panic)
}

// Driver is implemented separately by gtk4 and win32. Except Wake, every method
// must run on the pinned UI thread. Methods return native failures as errors.
// Init may partially allocate: Close must work even after Init fails.
// The event sink is synchronous and may cause nested native signals; adapters
// guard every FFI entry using Capture and defer cleanup until Run has returned.
// See docs/backend-contract.md for ownership, readiness, and teardown rules.
type Driver interface {
	// PrepareTree starts a delivery fence and owns hidden attempt resources.
	// A non-nil transaction MUST be returned with any error needing rollback
	// or cleanup. nil+error certifies no state change and no caller-owned work.
	// Call ApplyTree to complete every returned transaction on the UI thread.
	PrepareTree(TreeChange) (TreeTransaction, error)
	// SnapshotInput reads effective native Edit state without changing it.
	// PrepareTree must snapshot all affected input itself before mutation;
	// this hook also lets core retain detached state without native types.
	SnapshotInput(Peer) (InputState, error)
	Init(Callbacks) error
	Create(Config) (Peer, error)
	Destroy(Peer) error
	SetText(Peer, string) error
	Text(Peer) (string, error)
	Show(Peer) error
	// Run services native events and calls dispatch at safe loop turns, never
	// inside an event sink or a native operation reentered by an application callback.
	// Captured callback panic must be returned as data alongside a native error.
	Run(dispatch func() *Panic) (*Panic, error)
	// Quit requests exit without destroying resources or invoking application code.
	Quit()
	// Wake schedules a future loop turn. Safe across goroutines, nonblocking,
	// never inline dispatch. Valid after Init succeeds until queue Stop returns.
	Wake() error
	// Close releases infrastructure after all peers have been attempted; no
	// callbacks may reference the Go event sink once Close has returned.
	Close() error
}
