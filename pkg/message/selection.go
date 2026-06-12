package message

import (
	"strings"

	"github.com/inbucket/inbucket/v3/pkg/extension/event"
)

// MessageSelection describes optional filtering and pagination criteria for listing message
// metadata. The zero value selects every message in its original order, preserving the historical
// "return everything" behavior.
type MessageSelection struct {
	// Start is the zero-based offset of the first result to return.
	Start int
	// Limit is the maximum number of results to return; a value <= 0 means no limit.
	Limit int
	// Subject, when non-empty, keeps only messages whose subject contains it (case-insensitive
	// substring match).
	Subject string
	// Address, when non-empty, keeps only messages whose From or any To address contains it
	// (case-insensitive substring match).
	Address string
	// Seen, when non-nil, keeps only messages with a matching read status.
	Seen *bool
}

// Apply filters metas according to the selection criteria (AND semantics) and then applies
// Start/Limit pagination. It returns the requested page along with total, the number of messages
// that matched the filters before pagination (useful for reporting a total count to clients). The
// returned page slice is never nil. The input slice and its elements are not modified.
func (sel MessageSelection) Apply(
	metas []*event.MessageMetadata,
) (page []*event.MessageMetadata, total int) {
	subject := strings.ToLower(sel.Subject)
	address := strings.ToLower(sel.Address)

	filtered := make([]*event.MessageMetadata, 0, len(metas))
	for _, m := range metas {
		if sel.Seen != nil && m.Seen != *sel.Seen {
			continue
		}
		if subject != "" && !strings.Contains(strings.ToLower(m.Subject), subject) {
			continue
		}
		if address != "" && !addressContains(m, address) {
			continue
		}
		filtered = append(filtered, m)
	}
	total = len(filtered)

	// Apply pagination, clamping Start into [0, total] so out-of-range offsets yield an empty
	// page rather than panicking.
	start := sel.Start
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := total
	if sel.Limit > 0 && start+sel.Limit < end {
		end = start + sel.Limit
	}

	return filtered[start:end], total
}

// addressContains reports whether the message's From or any To address contains needle, which must
// already be lower-cased. Matching is performed against the RFC 5322 string form, so it covers both
// the display name and the address itself.
func addressContains(m *event.MessageMetadata, needle string) bool {
	if m.From != nil && strings.Contains(strings.ToLower(m.From.String()), needle) {
		return true
	}
	for _, to := range m.To {
		if to != nil && strings.Contains(strings.ToLower(to.String()), needle) {
			return true
		}
	}
	return false
}
