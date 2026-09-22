// SPDX-License-Identifier: TODO

package testutil

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// ErrExit is the error FakeRunner.Fail scripts, standing in for a non-zero exit.
var ErrExit = errors.New("exit status 1")

// Call records one Runner invocation.
type Call struct {
	Name string
	Env  []string
	Args []string
}

// String renders the call as a shell-like line, e.g. "systemctl --user start a.service".
func (c Call) String() string {
	return strings.Join(append([]string{c.Name}, c.Args...), " ")
}

// Result is a scripted outcome for FakeRunner.
type Result struct {
	Stdout string
	Stderr string
	Err    error
}

// FakeRunner implements adapters.Runner. Results are looked up by the exact
// Call.String() first, then by command name; unknown calls succeed with
// empty output.
type FakeRunner struct {
	mu      sync.Mutex
	calls   []Call
	results map[string]Result
	// Handler, when set, computes the result for calls without a scripted one.
	Handler func(Call) (Result, bool)
}

// NewFakeRunner returns an empty FakeRunner.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{results: map[string]Result{}, calls: nil, Handler: nil, mu: sync.Mutex{}}
}

// Script sets the result for a call line (as Call.String()) or a bare
// command name.
func (f *FakeRunner) Script(line string, r Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[line] = r
}

// Fail scripts an error for line with the given stderr.
func (f *FakeRunner) Fail(line, stderr string) {
	f.Script(line, Result{Stdout: "", Stderr: stderr, Err: ErrExit})
}

// Run implements adapters.Runner.
func (f *FakeRunner) Run(_ context.Context, name string, env []string, args ...string) ([]byte, []byte, error) {
	call := Call{Name: name, Env: env, Args: args}

	f.mu.Lock()
	f.calls = append(f.calls, call)
	r, ok := f.results[call.String()]
	if !ok {
		r, ok = f.results[name]
	}
	f.mu.Unlock()

	if !ok && f.Handler != nil {
		r, _ = f.Handler(call)
	}

	return []byte(r.Stdout), []byte(r.Stderr), r.Err
}

// Calls returns a copy of the recorded calls.
func (f *FakeRunner) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Lines returns the recorded calls as Call.String() values, in order.
func (f *FakeRunner) Lines() []string {
	calls := f.Calls()
	lines := make([]string, 0, len(calls))
	for _, c := range calls {
		lines = append(lines, c.String())
	}
	return lines
}

// Reset forgets recorded calls but keeps scripted results.
func (f *FakeRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}
