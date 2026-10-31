package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dop251/goja"
)

const (
	loadTimeout    = 5 * time.Second
	maxJSCallStack = 2048
	acquireTimeout = 5 * time.Second
)

var (
	// ErrTimeout means a plugin call ran past its deadline.
	ErrTimeout = errors.New("plugin call timed out")
	// ErrBusy means every VM in a plugin's pool stayed in use.
	ErrBusy = errors.New("plugin is busy")
)

// vm is one goja runtime and the handlers its script registered. goja runtimes
// aren't goroutine-safe, so a vm is only ever held by one call at a time.
type vm struct {
	rt       *goja.Runtime
	plugin   *Plugin
	ctx      context.Context
	channels map[string]*goja.Object
	webhooks map[string]*goja.Object
	admin    map[string]goja.Callable
	tools    map[string]goja.Callable
}

// Plugin is a loaded plugin with its bounded VM pool.
type Plugin struct {
	Manifest *Manifest
	Dir      string

	mgr     *Manager
	program *goja.Program
	timeout time.Duration
	// slots holds one token per allowed VM. A nil token creates a VM on demand,
	// which is also how a discarded VM gets replaced.
	slots chan *vm
}

func newPlugin(mgr *Manager, man *Manifest, dir string) (*Plugin, error) {
	program, err := compileEntry(dir, man)
	if err != nil {
		return nil, err
	}

	p := &Plugin{
		Manifest: man,
		Dir:      dir,
		mgr:      mgr,
		program:  program,
		timeout:  time.Duration(man.Server.TimeoutMs) * time.Millisecond,
		slots:    make(chan *vm, man.Server.Concurrency),
	}

	// Build one VM now, so a broken script or a missing handler fails the load.
	first, err := p.newVM()
	if err != nil {
		return nil, err
	}
	if err := first.checkHandlers(); err != nil {
		return nil, err
	}
	p.slots <- first
	for i := 1; i < man.Server.Concurrency; i++ {
		p.slots <- nil
	}
	return p, nil
}

func compileEntry(dir string, man *Manifest) (*goja.Program, error) {
	src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(man.Server.Entry)))
	if err != nil {
		return nil, fmt.Errorf("read server entry: %w", err)
	}
	program, err := goja.Compile(man.Server.Entry, string(src), false)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", man.Server.Entry, err)
	}
	return program, nil
}

func (p *Plugin) newVM() (*vm, error) {
	v := &vm{
		rt:       goja.New(),
		plugin:   p,
		ctx:      context.Background(),
		channels: map[string]*goja.Object{},
		webhooks: map[string]*goja.Object{},
		admin:    map[string]goja.Callable{},
		tools:    map[string]goja.Callable{},
	}
	v.rt.SetMaxCallStackSize(maxJSCallStack)
	if err := v.installGlobals(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()
	v.ctx = ctx
	stop := context.AfterFunc(ctx, func() { v.rt.Interrupt(ErrTimeout) })
	defer stop()

	if _, err := v.rt.RunProgram(p.program); err != nil {
		return nil, fmt.Errorf("run %s: %w", p.Manifest.Server.Entry, normalizeJSError(err))
	}
	return v, nil
}

// checkHandlers ties what the script registered to what the manifest declared.
func (v *vm) checkHandlers() error {
	man := v.plugin.Manifest
	for _, c := range man.Channels {
		if _, ok := v.channels[c.ID]; !ok {
			return fmt.Errorf("channel %q has no handler", c.ID)
		}
	}
	for _, w := range man.Webhooks {
		obj, ok := v.webhooks[w.Path]
		if !ok {
			return fmt.Errorf("webhook %q has no handler", w.Path)
		}
		if _, ok := goja.AssertFunction(obj.Get("handle")); !ok {
			return fmt.Errorf("webhook %q has no handle function", w.Path)
		}
		if w.Verify != verifyNone {
			if _, ok := goja.AssertFunction(obj.Get("verify")); !ok {
				return fmt.Errorf(`webhook %q has no verify function; declare "verify": "none" to allow that`, w.Path)
			}
		}
	}
	for _, r := range man.AdminRoutes {
		if _, ok := v.admin[r.Path]; !ok {
			return fmt.Errorf("admin route %q has no handler", r.Path)
		}
	}
	for _, t := range man.Tools {
		if _, ok := v.tools[t.Name]; !ok {
			return fmt.Errorf("tool %q has no handler", t.Name)
		}
	}
	return nil
}

// acquire takes a VM slot, waiting until ctx ends.
func (p *Plugin) acquire(ctx context.Context) (*vm, error) {
	select {
	case v := <-p.slots:
		if v != nil {
			return v, nil
		}
		v, err := p.newVM()
		if err != nil {
			p.slots <- nil
			return nil, err
		}
		return v, nil
	case <-ctx.Done():
		return nil, ErrBusy
	}
}

func (p *Plugin) release(v *vm, discard bool) {
	if discard {
		v = nil
	}
	p.slots <- v
}

// call runs fn on a pooled VM under the plugin's deadline. It settles any returned
// promise and converts the result to plain Go values, so JS objects never leave.
func (p *Plugin) call(ctx context.Context, fn func(*vm) (goja.Value, error)) (out any, err error) {
	v, err := p.acquire(ctx)
	if err != nil {
		return nil, err
	}

	discard := false
	defer func() {
		if r := recover(); r != nil {
			discard = true
			err = fmt.Errorf("plugin %s panicked: %v", p.Manifest.ID, r)
		}
		p.release(v, discard)
	}()

	callCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	v.ctx = callCtx
	stop := context.AfterFunc(callCtx, func() { v.rt.Interrupt(ErrTimeout) })

	res, runErr := fn(v)
	if !stop() {
		// The interrupt fired or is about to, and that VM can't be reused safely.
		discard = true
	}
	if runErr != nil {
		runErr = normalizeJSError(runErr)
		var interrupted *goja.InterruptedError
		if errors.As(runErr, &interrupted) || errors.Is(runErr, ErrTimeout) {
			discard = true
			return nil, ErrTimeout
		}
		return nil, runErr
	}
	return settle(res)
}

// settle unwraps an async handler's promise. There's no event loop, so a promise
// that's still pending when the handler returns can never finish.
func settle(val goja.Value) (any, error) {
	if val == nil {
		return nil, nil
	}
	if promise, ok := val.Export().(*goja.Promise); ok {
		switch promise.State() {
		case goja.PromiseStateFulfilled:
			val = promise.Result()
		case goja.PromiseStateRejected:
			return nil, rejectionError(promise.Result())
		default:
			return nil, errors.New("handler returned a promise that never settled; host calls are synchronous, so remove any waits on timers")
		}
	}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}
	return val.Export(), nil
}

func rejectionError(val goja.Value) error {
	if obj, ok := val.(*goja.Object); ok {
		if inner := obj.Get("value"); inner != nil {
			if err, ok := inner.Export().(error); ok {
				return err
			}
		}
	}
	return fmt.Errorf("promise rejected: %v", val)
}

// normalizeJSError surfaces a Go error thrown by a host call when there is one.
func normalizeJSError(err error) error {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		if inner := ex.Unwrap(); inner != nil {
			if errors.Is(inner, context.DeadlineExceeded) {
				return ErrTimeout
			}
			return inner
		}
	}
	return err
}
