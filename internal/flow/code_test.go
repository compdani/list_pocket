package flow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCodeLimits(t *testing.T) {
	cases := []struct{ name, code, want string }{
		{"no require", "return require('fs');", "require is not defined"},
		{"no process", "return process.env;", "process is not defined"},
		{"no fetch", "return fetch('http://example.com');", "fetch is not defined"},
		{"runaway loop", "while (true) {}", "limit"},
		{"catching doesn't stop the timeout", "while (true) { try { while (true) {} } catch (e) {} }", "limit"},
		{"runaway loop with logging", "while (true) { console.log({toJSON() { return 1; }}); }", "limit"},
		{"deep recursion", "function f() { return f(); } return f();", "call stack"},
		{"cyclic output", "const a = {}; a.a = a; return a;", "ircular"},
		{"function output", "return () => 1;", "JSON data"},
		{"huge output", "return 'x'.repeat(2 * 1024 * 1024);", "limit"},
		{"pending promise", "return new Promise(() => {});", "never settled"},
		{"rejected promise", "return Promise.reject(new Error('nope'));", "nope"},
		{"error line number", "\n\nnotDefined();", "code:3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, err := runCode(context.Background(), c.code, map[string]any{}, func(string) {})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("out = %v, err = %v, want an error with %q", out, err, c.want)
			}
		})
	}
}

func TestCodeStopsWhenRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err := runCode(ctx, "while (true) {}", map[string]any{}, func(string) {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
}

func TestCodeConsoleKeepsTheFirstLines(t *testing.T) {
	var lines []string
	_, err := runCode(context.Background(), "console.log('a', {b: 1}, 2); for (let i = 0; i < 500; i++) console.warn('x'.repeat(5000));", map[string]any{}, func(l string) {
		lines = append(lines, l)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != maxCodeLogLines || lines[0] != `a {"b":1} 2` || len(lines[1]) != maxCodeLogLineLen {
		t.Errorf("%d lines, first %q", len(lines), lines[0])
	}
}
