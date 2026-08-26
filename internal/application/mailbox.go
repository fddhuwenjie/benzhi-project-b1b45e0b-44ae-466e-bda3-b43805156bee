package application

import (
	"context"
	"sync"
)

type task struct {
	ctx  context.Context
	run  func() (CommandResult, error)
	done chan taskResult
}
type taskResult struct {
	result CommandResult
	err    error
}
type mailbox struct{ jobs chan task }

type mailboxes struct {
	values   sync.Map
	capacity int
}

func newMailboxes(capacity int) *mailboxes { return &mailboxes{capacity: capacity} }
func (m *mailboxes) get(caseID string) *mailbox {
	value, loaded := m.values.LoadOrStore(caseID, &mailbox{jobs: make(chan task, m.capacity)})
	box := value.(*mailbox)
	if !loaded {
		go box.loop()
	}
	return box
}
func (m *mailboxes) submit(ctx context.Context, caseID string, run func() (CommandResult, error)) (CommandResult, error) {
	t := task{ctx: ctx, run: run, done: make(chan taskResult, 1)}
	select {
	case m.get(caseID).jobs <- t:
	case <-ctx.Done():
		return CommandResult{}, ctx.Err()
	}
	select {
	case r := <-t.done:
		return r.result, r.err
	case <-ctx.Done():
		return CommandResult{}, ctx.Err()
	}
}
func (m *mailbox) loop() {
	for t := range m.jobs {
		select {
		case <-t.ctx.Done():
			t.done <- taskResult{err: t.ctx.Err()}
		default:
			r, e := t.run()
			t.done <- taskResult{result: r, err: e}
		}
	}
}
