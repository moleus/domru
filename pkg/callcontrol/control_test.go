package callcontrol

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type fakeCalls struct {
	current string
	ops     []string
	fail    bool
}

func (f *fakeCalls) Current() string { return f.current }
func (f *fakeCalls) Reject(id string) error {
	f.ops = append(f.ops, "reject:"+id)
	if id != f.current || f.fail {
		return errors.New("gone")
	}
	return nil
}
func (f *fakeCalls) Answer(_ context.Context, id string) error {
	f.ops = append(f.ops, "answer:"+id)
	return nil
}
func (f *fakeCalls) Bye(ctx context.Context, id string) error {
	f.ops = append(f.ops, "bye:"+id)
	return ctx.Err()
}
func TestOldButtonDoesNotTouchNewCall(t *testing.T) {
	f := &fakeCalls{current: "new"}
	opens := 0
	c := Controller{Calls: f, Mode: "reject", OpenDoor: func(context.Context) error { opens++; return nil }}
	old := "old"
	for i := 0; i < 2; i++ {
		r := c.Open(context.Background(), &old)
		require.Equal(t, Result{"accepted", "absent"}, r)
	}
	require.Equal(t, 2, opens)
	require.Empty(t, f.ops)
}
func TestCallReplacementDuringOpening(t *testing.T) {
	f := &fakeCalls{current: "old"}
	c := Controller{Calls: f, Mode: "reject", OpenDoor: func(context.Context) error { f.current = "new"; return nil }}
	r := c.Open(context.Background(), nil)
	require.Equal(t, Result{"accepted", "failed"}, r)
	require.Equal(t, []string{"reject:old"}, f.ops)
}
func TestAcceptedDialogAlwaysReleased(t *testing.T) {
	f := &fakeCalls{current: "call"}
	ctx, cancel := context.WithCancel(context.Background())
	c := Controller{Calls: f, Mode: "answer-bye", OpenDoor: func(context.Context) error { cancel(); f.ops = append(f.ops, "open"); return errors.New("lost") }}
	r := c.Open(ctx, nil)
	require.Equal(t, Result{"unknown", "ended"}, r)
	require.Equal(t, []string{"answer:call", "open", "bye:call"}, f.ops)
}
func TestSimultaneousOpenIsNotQueued(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c := Controller{Mode: "off", OpenDoor: func(context.Context) error { close(entered); <-release; return nil }}
	go func() { c.Open(context.Background(), nil); close(done) }()
	<-entered
	require.Equal(t, "busy", c.Open(context.Background(), nil).Opening)
	close(release)
	<-done
}
