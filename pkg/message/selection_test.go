package message_test

import (
	"fmt"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/inbucket/inbucket/v3/pkg/config"
	"github.com/inbucket/inbucket/v3/pkg/extension"
	"github.com/inbucket/inbucket/v3/pkg/extension/event"
	"github.com/inbucket/inbucket/v3/pkg/message"
	"github.com/inbucket/inbucket/v3/pkg/storage"
	"github.com/inbucket/inbucket/v3/pkg/storage/file"
	"github.com/inbucket/inbucket/v3/pkg/storage/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

// meta builds a metadata record for selection unit tests.
func meta(id, subject, from, to string, seen bool) *event.MessageMetadata {
	return &event.MessageMetadata{
		ID:      id,
		Subject: subject,
		From:    &mail.Address{Address: from},
		To:      []*mail.Address{{Address: to}},
		Seen:    seen,
	}
}

// ids extracts the ordered message IDs from a metadata slice.
func ids(metas []*event.MessageMetadata) []string {
	out := make([]string, len(metas))
	for i, m := range metas {
		out[i] = m.ID
	}
	return out
}

func TestMessageSelectionApply(t *testing.T) {
	// Fixed dataset, in delivery (insertion) order.
	dataset := []*event.MessageMetadata{
		meta("1", "Alpha Invoice", "alice@acme.com", "bob@corp.com", false),
		meta("2", "Beta Report", "carol@globex.com", "bob@corp.com", true),
		meta("3", "Gamma invoice", "dave@acme.com", "eve@corp.com", false),
		meta("4", "Delta Update", "alice@acme.com", "bob@corp.com", true),
		meta("5", "Epsilon Note", "frank@initech.com", "bob@corp.com", false),
	}

	tests := []struct {
		name      string
		sel       message.MessageSelection
		wantIDs   []string
		wantTotal int
	}{
		// No criteria: everything, in order.
		{"all", message.MessageSelection{}, []string{"1", "2", "3", "4", "5"}, 5},

		// Read-status filter.
		{"unseen", message.MessageSelection{Seen: boolPtr(false)}, []string{"1", "3", "5"}, 3},
		{"seen", message.MessageSelection{Seen: boolPtr(true)}, []string{"2", "4"}, 2},

		// Subject filter (case-insensitive substring).
		{"subject", message.MessageSelection{Subject: "invoice"}, []string{"1", "3"}, 2},
		{"subject upper", message.MessageSelection{Subject: "INVOICE"}, []string{"1", "3"}, 2},
		{"subject none", message.MessageSelection{Subject: "zzz"}, []string{}, 0},

		// Address filter matches From or any To, case-insensitive.
		{"address from", message.MessageSelection{Address: "acme.com"}, []string{"1", "3", "4"}, 3},
		{"address to", message.MessageSelection{Address: "eve@corp"}, []string{"3"}, 1},
		{"address upper", message.MessageSelection{Address: "ACME.COM"}, []string{"1", "3", "4"}, 3},

		// Combined filters (AND semantics).
		{"unseen+subject",
			message.MessageSelection{Seen: boolPtr(false), Subject: "invoice"},
			[]string{"1", "3"}, 2},
		{"unseen+address",
			message.MessageSelection{Seen: boolPtr(false), Address: "acme.com"},
			[]string{"1", "3"}, 2},
		{"unseen+subject+address",
			message.MessageSelection{Seen: boolPtr(false), Subject: "invoice", Address: "acme.com"},
			[]string{"1", "3"}, 2},
		{"seen+subject empty",
			message.MessageSelection{Seen: boolPtr(true), Subject: "invoice"},
			[]string{}, 0},

		// Pagination boundaries (no filter, total 5).
		{"page first", message.MessageSelection{Start: 0, Limit: 2}, []string{"1", "2"}, 5},
		{"page middle", message.MessageSelection{Start: 2, Limit: 2}, []string{"3", "4"}, 5},
		{"page last partial", message.MessageSelection{Start: 4, Limit: 2}, []string{"5"}, 5},
		{"start at end", message.MessageSelection{Start: 5, Limit: 2}, []string{}, 5},
		{"start past end", message.MessageSelection{Start: 10, Limit: 2}, []string{}, 5},
		{"limit zero is unlimited",
			message.MessageSelection{Start: 0, Limit: 0},
			[]string{"1", "2", "3", "4", "5"}, 5},
		{"limit exceeds remaining",
			message.MessageSelection{Start: 0, Limit: 99},
			[]string{"1", "2", "3", "4", "5"}, 5},
		{"offset with no limit", message.MessageSelection{Start: 2, Limit: 0}, []string{"3", "4", "5"}, 5},

		// Combined filter + pagination: paginate over the filtered subset; total reflects the filter.
		{"unseen page two",
			message.MessageSelection{Seen: boolPtr(false), Start: 1, Limit: 1},
			[]string{"3"}, 3},
		{"unseen first two",
			message.MessageSelection{Seen: boolPtr(false), Start: 0, Limit: 2},
			[]string{"1", "3"}, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, total := tc.sel.Apply(dataset)
			assert.Equal(t, tc.wantIDs, ids(page), "page IDs")
			assert.Equal(t, tc.wantTotal, total, "total")
			assert.NotNil(t, page, "page must never be nil")
		})
	}

	// The input slice must not be mutated by filtering/pagination.
	require.Len(t, dataset, 5, "Apply must not modify the input slice")
}

// deliverMsg delivers a message to store, marking it seen when requested.
func deliverMsg(t *testing.T, store storage.Store, mailbox, from, to, subject string, seen bool) {
	t.Helper()
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nbody\r\n", from, to, subject)
	delivery := &message.Delivery{
		Meta: event.MessageMetadata{
			Mailbox: mailbox,
			From:    &mail.Address{Address: from},
			To:      []*mail.Address{{Address: to}},
			Date:    time.Now(),
			Subject: subject,
		},
		Reader: strings.NewReader(body),
	}
	id, err := store.AddMessage(delivery)
	require.NoError(t, err, "AddMessage failed")
	if seen {
		require.NoError(t, store.MarkSeen(mailbox, id), "MarkSeen failed")
	}
}

// row is a backend-independent projection used to compare results across stores, since the mem and
// file backends assign different message IDs.
type row struct {
	Subject string
	From    string
	Seen    bool
}

func project(metas []*event.MessageMetadata) []row {
	out := make([]row, len(metas))
	for i, m := range metas {
		from := ""
		if m.From != nil {
			from = m.From.Address
		}
		out[i] = row{Subject: m.Subject, From: from, Seen: m.Seen}
	}
	return out
}

// TestMessageSelectionCrossBackend delivers an identical message sequence to the mem and file
// stores and verifies that every filtered/paginated query yields identical results, regardless of
// backend.
func TestMessageSelectionCrossBackend(t *testing.T) {
	host := extension.NewHost()
	memStore, err := mem.New(config.Storage{}, host)
	require.NoError(t, err, "mem.New failed")
	fileStore, err := file.New(config.Storage{Params: map[string]string{"path": t.TempDir()}}, host)
	require.NoError(t, err, "file.New failed")
	backends := map[string]storage.Store{"mem": memStore, "file": fileStore}

	const mailbox = "parity"
	sequence := []struct {
		from, to, subject string
		seen              bool
	}{
		{"alice@acme.com", "bob@corp.com", "Alpha Invoice", false},
		{"carol@globex.com", "bob@corp.com", "Beta Report", true},
		{"dave@acme.com", "eve@corp.com", "Gamma invoice", false},
		{"alice@acme.com", "bob@corp.com", "Delta Update", true},
		{"frank@initech.com", "bob@corp.com", "Epsilon Note", false},
	}
	for _, store := range backends {
		for _, m := range sequence {
			deliverMsg(t, store, mailbox, m.from, m.to, m.subject, m.seen)
		}
	}

	selections := []struct {
		name      string
		sel       message.MessageSelection
		want      []row
		wantTotal int
	}{
		{"all", message.MessageSelection{}, []row{
			{"Alpha Invoice", "alice@acme.com", false},
			{"Beta Report", "carol@globex.com", true},
			{"Gamma invoice", "dave@acme.com", false},
			{"Delta Update", "alice@acme.com", true},
			{"Epsilon Note", "frank@initech.com", false},
		}, 5},
		{"unseen", message.MessageSelection{Seen: boolPtr(false)}, []row{
			{"Alpha Invoice", "alice@acme.com", false},
			{"Gamma invoice", "dave@acme.com", false},
			{"Epsilon Note", "frank@initech.com", false},
		}, 3},
		{"subject invoice", message.MessageSelection{Subject: "invoice"}, []row{
			{"Alpha Invoice", "alice@acme.com", false},
			{"Gamma invoice", "dave@acme.com", false},
		}, 2},
		{"address acme", message.MessageSelection{Address: "acme.com"}, []row{
			{"Alpha Invoice", "alice@acme.com", false},
			{"Gamma invoice", "dave@acme.com", false},
			{"Delta Update", "alice@acme.com", true},
		}, 3},
		{"unseen+subject", message.MessageSelection{Seen: boolPtr(false), Subject: "invoice"}, []row{
			{"Alpha Invoice", "alice@acme.com", false},
			{"Gamma invoice", "dave@acme.com", false},
		}, 2},
		{"paginated", message.MessageSelection{Start: 1, Limit: 2}, []row{
			{"Beta Report", "carol@globex.com", true},
			{"Gamma invoice", "dave@acme.com", false},
		}, 5},
		{"unseen paginated", message.MessageSelection{Seen: boolPtr(false), Start: 1, Limit: 1}, []row{
			{"Gamma invoice", "dave@acme.com", false},
		}, 3},
	}

	for _, sc := range selections {
		t.Run(sc.name, func(t *testing.T) {
			results := map[string][]row{}
			totals := map[string]int{}
			for name, store := range backends {
				sm := &message.StoreManager{Store: store, ExtHost: host}
				metas, err := sm.GetMetadata(mailbox)
				require.NoError(t, err, "GetMetadata(%s)", name)
				page, total := sc.sel.Apply(metas)
				results[name] = project(page)
				totals[name] = total
			}

			// Each backend produces the expected result.
			assert.Equal(t, sc.want, results["mem"], "mem result")
			assert.Equal(t, sc.want, results["file"], "file result")
			assert.Equal(t, sc.wantTotal, totals["mem"], "mem total")
			assert.Equal(t, sc.wantTotal, totals["file"], "file total")

			// The two backends agree with each other: the core consistency guarantee.
			assert.Equal(t, results["mem"], results["file"], "mem vs file result parity")
			assert.Equal(t, totals["mem"], totals["file"], "mem vs file total parity")
		})
	}
}

// TestMessageSelectionRealtimeDelivery verifies that filtered/paginated queries immediately and
// consistently reflect messages as they are delivered, with no stale or cached results.
func TestMessageSelectionRealtimeDelivery(t *testing.T) {
	host := extension.NewHost()
	store, err := mem.New(config.Storage{}, host)
	require.NoError(t, err, "mem.New failed")
	sm := &message.StoreManager{Store: store, ExtHost: host}
	const mailbox = "live"

	query := func(sel message.MessageSelection) (subjects []string, total int) {
		metas, err := sm.GetMetadata(mailbox)
		require.NoError(t, err, "GetMetadata failed")
		page, total := sel.Apply(metas)
		subjects = make([]string, len(page))
		for i, m := range page {
			subjects[i] = m.Subject
		}
		return subjects, total
	}

	unseen := message.MessageSelection{Seen: boolPtr(false)}

	// A freshly delivered message is immediately visible to a filtered query.
	deliverMsg(t, store, mailbox, "a@x.com", "u@x.com", "one", false)
	subs, total := query(unseen)
	assert.Equal(t, []string{"one"}, subs)
	assert.Equal(t, 1, total)

	// A seen message is excluded from the unseen filter but visible to the seen filter.
	deliverMsg(t, store, mailbox, "b@x.com", "u@x.com", "two", true)
	subs, total = query(unseen)
	assert.Equal(t, []string{"one"}, subs, "seen message must not appear in unseen filter")
	assert.Equal(t, 1, total)
	subs, total = query(message.MessageSelection{Seen: boolPtr(true)})
	assert.Equal(t, []string{"two"}, subs)
	assert.Equal(t, 1, total)

	// Subsequent deliveries are reflected, and pagination spans the live data set.
	deliverMsg(t, store, mailbox, "c@x.com", "u@x.com", "three", false)
	subs, total = query(unseen)
	assert.Equal(t, []string{"one", "three"}, subs)
	assert.Equal(t, 2, total)

	// Page two (offset 1, size 1) over the unseen subset returns the newest unseen message.
	subs, total = query(message.MessageSelection{Seen: boolPtr(false), Start: 1, Limit: 1})
	assert.Equal(t, []string{"three"}, subs)
	assert.Equal(t, 2, total)
}
