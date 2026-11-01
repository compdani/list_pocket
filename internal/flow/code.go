package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const (
	codeNodeTimeout    = 10 * time.Second
	codeAbandonGrace   = time.Second
	maxCodeCallStack   = 2048
	maxCodeOutputBytes = 1 << 20
	maxCodeLogLines    = 100
	maxCodeLogLineLen  = 2000
)

// Fields a body or loop adds to the environment. They always exist in code, so reading
// one outside a body gives undefined rather than a ReferenceError.
var codeOptionalGlobals = []string{"item", "index", "loop"}

// compileCode wraps a code node's source in a function, so it can use return. The source
// starts on the wrapper's first line, which keeps line numbers in errors right.
func compileCode(src string) (*goja.Program, error) {
	return goja.Compile("code", "(function () {"+src+"\n})", false)
}

// runCode runs a code node's source in a new VM with the run data as globals. The VM has
// no access to the network, files, or the rest of the app. A VM can't be limited to an
// amount of memory, so code that allocates without stopping is only limited by the timeout.
func runCode(ctx context.Context, src string, env map[string]any, logf func(string)) (any, error) {
	program, err := compileCode(src)
	if err != nil {
		return nil, err
	}
	// The VM gets its own copy. Run data is shared with ForEach items running at the same time.
	envJSON, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, codeNodeTimeout)
	defer cancel()
	rt := goja.New()
	stop := context.AfterFunc(runCtx, func() { rt.Interrupt("code timed out") })
	defer stop()

	type result struct {
		out any
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("code panicked: %v", r)}
			}
		}()
		out, err := evalCode(rt, program, envJSON, logf)
		done <- result{out, err}
	}()

	var res result
	select {
	case res = <-done:
	case <-runCtx.Done():
		// Interrupts are seen between instructions, so a slow native call such as a
		// regexp can outlive the deadline. Don't hold up the run for it.
		select {
		case res = <-done:
		case <-time.After(codeAbandonGrace):
			return nil, codeStopped(ctx)
		}
	}
	var interrupted *goja.InterruptedError
	if errors.As(res.err, &interrupted) {
		return nil, codeStopped(ctx)
	}
	// This one has no message of its own.
	var overflow *goja.StackOverflowError
	if errors.As(res.err, &overflow) {
		return nil, errors.New("code exceeded the call stack limit; check for runaway recursion")
	}
	return res.out, res.err
}

// codeStopped keeps a cancelled run looking cancelled rather than failed.
func codeStopped(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("code ran past its %s limit", codeNodeTimeout)
}

func evalCode(rt *goja.Runtime, program *goja.Program, envJSON []byte, logf func(string)) (any, error) {
	rt.SetMaxCallStackSize(maxCodeCallStack)

	// Taken before any code runs, which could replace them.
	jsonObj := rt.Get("JSON").ToObject(rt)
	parse, _ := goja.AssertFunction(jsonObj.Get("parse"))
	stringify, _ := goja.AssertFunction(jsonObj.Get("stringify"))

	parsed, err := parse(goja.Undefined(), rt.ToValue(string(envJSON)))
	if err != nil {
		return nil, err
	}
	for _, name := range codeOptionalGlobals {
		if err := rt.Set(name, goja.Undefined()); err != nil {
			return nil, err
		}
	}
	envObj := parsed.ToObject(rt)
	for _, name := range envObj.Keys() {
		if err := rt.Set(name, envObj.Get(name)); err != nil {
			return nil, err
		}
	}
	if err := rt.Set("console", codeConsole(rt, stringify, logf)); err != nil {
		return nil, err
	}

	wrapper, err := rt.RunProgram(program)
	if err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(wrapper)
	if !ok {
		return nil, errors.New("code didn't compile to a function")
	}
	val, err := fn(goja.Undefined())
	if err != nil {
		return nil, err
	}

	// There's no event loop, so a promise that's still pending can never finish.
	if promise, ok := val.Export().(*goja.Promise); ok {
		switch promise.State() {
		case goja.PromiseStateFulfilled:
			val = promise.Result()
		case goja.PromiseStateRejected:
			return nil, fmt.Errorf("code rejected: %v", promise.Result())
		default:
			return nil, errors.New("code returned a promise that never settled; there are no timers or network calls to wait for")
		}
	}
	if val == nil || goja.IsUndefined(val) {
		return nil, nil
	}
	text, err := stringify(goja.Undefined(), val)
	if err != nil {
		return nil, err
	}
	if goja.IsUndefined(text) {
		return nil, errors.New("code must return JSON data, not a function or symbol")
	}
	raw := text.String()
	if len(raw) > maxCodeOutputBytes {
		return nil, fmt.Errorf("code output is %d bytes; the limit is %d", len(raw), maxCodeOutputBytes)
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// codeConsole gives code the usual console calls, so they work as people expect. Only the
// first lines of a run are kept, so a loop can't flood the logs.
func codeConsole(rt *goja.Runtime, stringify goja.Callable, logf func(string)) *goja.Object {
	lines := 0
	log := func(call goja.FunctionCall) goja.Value {
		if lines >= maxCodeLogLines {
			return goja.Undefined()
		}
		lines++
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = formatCodeLogArg(arg, stringify)
		}
		line := strings.Join(parts, " ")
		if len(line) > maxCodeLogLineLen {
			line = line[:maxCodeLogLineLen]
		}
		logf(line)
		return goja.Undefined()
	}
	console := rt.NewObject()
	for _, name := range []string{"log", "info", "warn", "error", "debug"} {
		_ = console.Set(name, log)
	}
	return console
}

func formatCodeLogArg(arg goja.Value, stringify goja.Callable) string {
	if _, isString := arg.Export().(string); isString {
		return arg.String()
	}
	text, err := stringify(goja.Undefined(), arg)
	if err != nil {
		// An interrupt only fires once, so it has to go on stopping the code from here.
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			panic(interrupted)
		}
		return arg.String()
	}
	if goja.IsUndefined(text) {
		return arg.String()
	}
	return text.String()
}
