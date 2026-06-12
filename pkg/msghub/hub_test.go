package msghub

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/inbucket/inbucket/v3/pkg/extension"
	"github.com/inbucket/inbucket/v3/pkg/extension/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testListener implements the Listener interface, mock for unit tests
type testListener struct {
	messages   []*event.MessageMetadata // received messages
	deletes    []string                 // received deletes
	wantEvents int                      // how many events this listener wants to receive
	errorAfter int                      // when != 0, event count until Receive() begins returning error
	gotEvents  int

	done     chan struct{} // closed once we have received wantMessages
	overflow chan struct{} // closed if we receive wantMessages+1
}

func newTestListener(want int) *testListener {
	l := &testListener{
		messages:   make([]*event.MessageMetadata, 0, want*2),
		deletes:    make([]string, 0, want*2),
		wantEvents: want,
		done:       make(chan struct{}),
		overflow:   make(chan struct{}),
	}
	if want == 0 {
		close(l.done)
	}
	return l
}

// Receive a Message, store it in the messages slice, close applicable channels, and return an error
// if instructed
func (l *testListener) Receive(msg event.MessageMetadata) error {
	l.gotEvents++
	l.messages = append(l.messages, &msg)
	if l.gotEvents == l.wantEvents {
		close(l.done)
	}
	if l.gotEvents == l.wantEvents+1 {
		close(l.overflow)
	}
	if l.errorAfter > 0 && l.gotEvents > l.errorAfter {
		return errors.New("too many messages")
	}
	return nil
}

func (l *testListener) Delete(mailbox string, id string) error {
	l.gotEvents++
	l.deletes = append(l.deletes, mailbox+"/"+id)
	if l.gotEvents == l.wantEvents {
		close(l.done)
	}
	if l.gotEvents == l.wantEvents+1 {
		close(l.overflow)
	}
	return nil
}

// String formats the got vs wanted message counts
func (l *testListener) String() string {
	return fmt.Sprintf("got %v messages, wanted %v", len(l.messages), l.wantEvents)
}

func TestHubNew(t *testing.T) {
	hub := New(5, extension.NewHost())
	if hub == nil {
		t.Fatal("New() == nil, expected a new Hub")
	}
}

func TestHubZeroLen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}
	for range 100 {
		hub.Dispatch(m)
	}
	// Ensures Hub doesn't panic
}

func TestHubZeroListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}
	for range 100 {
		hub.Dispatch(m)
	}
	// Ensures Hub doesn't panic
}

func TestHubOneListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}
	l := newTestListener(1)

	hub.AddListener(l)
	hub.Dispatch(m)

	// Wait for messages
	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Error("Timeout:", l)
	}
}

func TestHubRemoveListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}
	l := newTestListener(1)

	hub.AddListener(l)
	hub.Dispatch(m)
	hub.RemoveListener(l)
	hub.Dispatch(m)
	hub.Sync()

	// Wait for messages
	select {
	case <-l.overflow:
		t.Error(l)
	case <-time.After(50 * time.Millisecond):
		// Expected result, no overflow
	}
}

func TestHubRemoveListenerOnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}

	// error after 1 means listener should receive 2 messages before being removed
	l := newTestListener(2)
	l.errorAfter = 1

	hub.AddListener(l)
	hub.Dispatch(m)
	hub.Dispatch(m)
	hub.Dispatch(m)
	hub.Dispatch(m)
	hub.Sync()

	// Wait for messages
	select {
	case <-l.overflow:
		t.Error(l)
	case <-time.After(50 * time.Millisecond):
		// Expected result, no overflow
	}
}

func TestHubHistoryReplay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(100, extension.NewHost())
	go hub.Start(ctx)
	l1 := newTestListener(3)
	hub.AddListener(l1)

	// Broadcast 3 messages with no listeners
	msgs := make([]event.MessageMetadata, 3)
	for i := range msgs {
		msgs[i] = event.MessageMetadata{
			Subject: fmt.Sprintf("subj %v", i),
		}
		hub.Dispatch(msgs[i])
	}

	// Wait for messages (live)
	select {
	case <-l1.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l1)
	}

	// Add a new listener
	l2 := newTestListener(3)
	hub.AddListener(l2)

	// Wait for messages (history)
	select {
	case <-l2.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l2)
	}

	for i := range msgs {
		got := l2.messages[i].Subject
		want := msgs[i].Subject
		if got != want {
			t.Errorf("msg[%v].Subject == %q, want %q", i, got, want)
		}
	}
}

func TestHubHistoryDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(100, extension.NewHost())
	go hub.Start(ctx)
	l1 := newTestListener(3)
	hub.AddListener(l1)

	// Broadcast 3 messages with no listeners
	msgs := make([]event.MessageMetadata, 3)
	for i := range msgs {
		msgs[i] = event.MessageMetadata{
			Mailbox: "hub",
			ID:      strconv.Itoa(i),
			Subject: fmt.Sprintf("subj %v", i),
		}
		hub.Dispatch(msgs[i])
	}

	// Wait for messages (live)
	select {
	case <-l1.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l1)
	}

	hub.Delete("hub", "1") // Delete a message
	hub.Delete("zzz", "0") // Attempt to delete non-existent mailbox message

	// Add a new listener, waits for 2 messages
	l2 := newTestListener(2)
	hub.AddListener(l2)

	// Wait for messages (history)
	select {
	case <-l2.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l2)
	}

	want := []string{"subj 0", "subj 2"}
	for i := range want {
		got := l2.messages[i].Subject
		if got != want[i] {
			t.Errorf("msg[%v].Subject == %q, want %q", i, got, want[i])
		}
	}
}

func TestHubHistoryReplayWrap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	l1 := newTestListener(20)
	hub.AddListener(l1)

	// Broadcast more messages than the hub can hold
	msgs := make([]event.MessageMetadata, 20)
	for i := range msgs {
		msgs[i] = event.MessageMetadata{
			Subject: fmt.Sprintf("subj %v", i),
		}
		hub.Dispatch(msgs[i])
	}

	// Wait for messages (live)
	select {
	case <-l1.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l1)
	}

	// Add a new listener
	l2 := newTestListener(5)
	hub.AddListener(l2)

	// Wait for messages (history)
	select {
	case <-l2.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout:", l2)
	}

	for i := range 5 {
		got := l2.messages[i].Subject
		want := msgs[i+15].Subject
		if got != want {
			t.Errorf("msg[%v].Subject == %q, want %q", i, got, want)
		}
	}
}

func TestHubHistoryReplayWrapAfterDelete(t *testing.T) {
	bufferSize := 5

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(bufferSize, extension.NewHost())
	go hub.Start(ctx)

	waitForMessages := func(n int) {
		l := newTestListener(n)
		hub.AddListener(l)

		select {
		case <-l.done:
		case <-time.After(time.Second):
			t.Fatal("Timeout:", l)
		}
	}

	// Broadcast more messages than the hub can hold.
	msgs := make([]event.MessageMetadata, 10)
	for i := range msgs {
		msgs[i] = event.MessageMetadata{
			Mailbox: "first",
			ID:      strconv.Itoa(i),
			Subject: fmt.Sprintf("subj %v", i),
		}
		hub.Dispatch(msgs[i])
	}
	waitForMessages(bufferSize)

	// Buffer must be configured size.
	require.Equal(t, bufferSize, hub.history.Len())

	// Delete a message still present in buffer.
	hub.Delete("first", "7")

	// Broadcast another set of messages.
	for i := range msgs {
		msgs[i] = event.MessageMetadata{
			Mailbox: "second",
			ID:      strconv.Itoa(i),
			Subject: fmt.Sprintf("subj %v", i),
		}
		hub.Dispatch(msgs[i])
	}
	waitForMessages(bufferSize)

	// Ensure the buffer did not shrink after delete.
	got := hub.history.Len()
	assert.Equal(t, bufferSize, got, "got buffer size %d, wanted %d", got, bufferSize)
}

func TestHubContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hub := New(5, extension.NewHost())
	go hub.Start(ctx)
	m := event.MessageMetadata{}
	l := newTestListener(1)

	hub.AddListener(l)
	hub.Dispatch(m)
	hub.Sync()
	cancel()

	// Wait for messages
	select {
	case <-l.overflow:
		t.Error(l)
	case <-time.After(50 * time.Millisecond):
		// Expected result, no overflow
	}
}

// --- Regression tests: zero-history real-time push ---
// https://github.com/inbucket/inbucket/issues/XXX
// Setting historyLen=0 should disable replay but still relay live events.

// TestHubZeroHistoryDispatchRelays verifies that with history disabled (historyLen=0),
// Dispatch still relays new messages to registered listeners (global monitor scenario).
func TestHubZeroHistoryDispatchRelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)

	l := newTestListener(3)
	hub.AddListener(l)

	// Dispatch 3 messages; all should be relayed even though history is disabled.
	for i := range 3 {
		hub.Dispatch(event.MessageMetadata{
			Subject: fmt.Sprintf("live-%d", i),
		})
	}

	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: listener did not receive live messages with zero history:", l)
	}

	for i := range 3 {
		got := l.messages[i].Subject
		want := fmt.Sprintf("live-%d", i)
		assert.Equal(t, want, got, "message %d subject mismatch", i)
	}
}

// TestHubZeroHistoryMailboxFilter verifies that with history disabled, a listener that
// filters by mailbox still receives only matching messages (single-mailbox monitor scenario).
func TestHubZeroHistoryMailboxFilter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)

	fl := &filteredListenerData{
		messages:   make([]*event.MessageMetadata, 0, 4),
		wantEvents: 2,
		done:       make(chan struct{}),
	}
	wrapper := &mailboxFilterListener{
		inner:   fl,
		mailbox: "inbox",
	}
	hub.AddListener(wrapper)

	// Dispatch messages to different mailboxes.
	hub.Dispatch(event.MessageMetadata{Mailbox: "inbox", Subject: "yes-1"})
	hub.Dispatch(event.MessageMetadata{Mailbox: "other", Subject: "no-1"})
	hub.Dispatch(event.MessageMetadata{Mailbox: "inbox", Subject: "yes-2"})
	hub.Dispatch(event.MessageMetadata{Mailbox: "other", Subject: "no-2"})

	select {
	case <-fl.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: filtered listener did not receive expected messages")
	}

	assert.Len(t, fl.messages, 2)
	assert.Equal(t, "yes-1", fl.messages[0].Subject)
	assert.Equal(t, "yes-2", fl.messages[1].Subject)
}

// mailboxFilterListener wraps a filteredListener and filters by mailbox,
// mirroring the behavior of msgListenerV1/V2 in the WebSocket controllers.
type mailboxFilterListener struct {
	inner   *filteredListenerData
	mailbox string
}

type filteredListenerData struct {
	messages   []*event.MessageMetadata
	wantEvents int
	gotEvents  int
	done       chan struct{}
}

func (ml *mailboxFilterListener) Receive(msg event.MessageMetadata) error {
	if ml.mailbox != "" && ml.mailbox != msg.Mailbox {
		return nil
	}
	ml.inner.gotEvents++
	ml.inner.messages = append(ml.inner.messages, &msg)
	if ml.inner.gotEvents == ml.inner.wantEvents {
		close(ml.inner.done)
	}
	return nil
}

func (ml *mailboxFilterListener) Delete(mailbox string, id string) error {
	return nil
}

// TestHubZeroHistoryDeleteRelays verifies that with history disabled,
// Delete events are still relayed to registered listeners.
func TestHubZeroHistoryDeleteRelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)

	// Listener expects 1 delete event.
	l := &testListener{
		messages:   make([]*event.MessageMetadata, 0, 4),
		deletes:    make([]string, 0, 4),
		wantEvents: 1,
		done:       make(chan struct{}),
		overflow:   make(chan struct{}),
	}
	hub.AddListener(l)

	hub.Delete("user1", "msg-42")

	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: listener did not receive delete event with zero history")
	}

	require.Len(t, l.deletes, 1)
	assert.Equal(t, "user1/msg-42", l.deletes[0])
}

// TestHubZeroHistoryReconnectRelays verifies that after a listener disconnects and
// reconnects with zero history, it still receives new live messages (reconnect scenario).
func TestHubZeroHistoryReconnectRelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)

	// Phase 1: first connection receives a message.
	l1 := newTestListener(1)
	hub.AddListener(l1)
	hub.Dispatch(event.MessageMetadata{Subject: "before-reconnect"})

	select {
	case <-l1.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: first listener did not receive message:", l1)
	}

	// Phase 2: disconnect (simulate WebSocket close).
	hub.RemoveListener(l1)
	hub.Sync()

	// Phase 3: reconnect with a new listener.
	l2 := newTestListener(1)
	hub.AddListener(l2)

	// No history replay expected (historyLen=0).
	// Dispatch a new message; the reconnected listener should receive it.
	hub.Dispatch(event.MessageMetadata{Subject: "after-reconnect"})

	select {
	case <-l2.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: reconnected listener did not receive live message:", l2)
	}

	assert.Equal(t, "after-reconnect", l2.messages[0].Subject)
	// Ensure no stale replay was delivered.
	assert.Len(t, l2.messages, 1, "reconnected listener should only get live message, no replay")
}

// TestHubZeroHistoryMixedEvents verifies that with zero history, a listener receives
// both Receive and Delete events in order during a mixed workload.
func TestHubZeroHistoryMixedEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := New(0, extension.NewHost())
	go hub.Start(ctx)

	// Expect 3 events total: 2 receives + 1 delete.
	l := &testListener{
		messages:   make([]*event.MessageMetadata, 0, 4),
		deletes:    make([]string, 0, 4),
		wantEvents: 3,
		done:       make(chan struct{}),
		overflow:   make(chan struct{}),
	}
	hub.AddListener(l)

	hub.Dispatch(event.MessageMetadata{Mailbox: "box", ID: "1", Subject: "first"})
	hub.Dispatch(event.MessageMetadata{Mailbox: "box", ID: "2", Subject: "second"})
	hub.Delete("box", "1")

	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("Timeout: listener did not receive mixed events with zero history:", l)
	}

	assert.Len(t, l.messages, 2)
	assert.Len(t, l.deletes, 1)
	assert.Equal(t, "first", l.messages[0].Subject)
	assert.Equal(t, "second", l.messages[1].Subject)
	assert.Equal(t, "box/1", l.deletes[0])
}
