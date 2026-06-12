package test

import (
	"errors"
	"sort"

	"github.com/inbucket/inbucket/v3/pkg/config"
	"github.com/inbucket/inbucket/v3/pkg/extension/event"
	"github.com/inbucket/inbucket/v3/pkg/message"
	"github.com/inbucket/inbucket/v3/pkg/policy"
	"github.com/inbucket/inbucket/v3/pkg/storage"
)

// ManagerStub is a test stub for message.Manager
type ManagerStub struct {
	message.Manager
	mailboxes map[string][]*message.Message
	// MailboxesErr, when set, is returned by GetMailboxes to simulate a storage failure.
	MailboxesErr error
}

// NewManager creates a new ManagerStub.
func NewManager() *ManagerStub {
	return &ManagerStub{
		mailboxes: make(map[string][]*message.Message),
	}
}

// AddMessage adds a message to the specified mailbox.
func (m *ManagerStub) AddMessage(mailbox string, msg *message.Message) {
	messages := m.mailboxes[mailbox]
	m.mailboxes[mailbox] = append(messages, msg)
}

// GetMessage gets a message by ID from the specified mailbox.
func (m *ManagerStub) GetMessage(mailbox, id string) (*message.Message, error) {
	if mailbox == "messageerr" {
		return nil, errors.New("internal error")
	}
	for _, msg := range m.mailboxes[mailbox] {
		if msg.ID == id {
			return msg, nil
		}
	}
	return nil, storage.ErrNotExist
}

// GetMetadata gets all the metadata for the specified mailbox.
func (m *ManagerStub) GetMetadata(mailbox string) ([]*event.MessageMetadata, error) {
	if mailbox == "messageserr" {
		return nil, errors.New("internal error")
	}
	messages := m.mailboxes[mailbox]
	metas := make([]*event.MessageMetadata, len(messages))
	for i, msg := range messages {
		metas[i] = &msg.MessageMetadata
	}
	return metas, nil
}

// GetMailboxes returns a summary of each active (non-empty) mailbox, sorted by name. Set
// MailboxesErr to simulate a storage failure.
func (m *ManagerStub) GetMailboxes() ([]*message.MailboxSummary, error) {
	if m.MailboxesErr != nil {
		return nil, m.MailboxesErr
	}
	summaries := make([]*message.MailboxSummary, 0, len(m.mailboxes))
	for name, msgs := range m.mailboxes {
		if len(msgs) == 0 {
			continue
		}
		unread := 0
		for _, msg := range msgs {
			if !msg.Seen {
				unread++
			}
		}
		latest := msgs[len(msgs)-1]
		summaries = append(summaries, &message.MailboxSummary{
			Name:   name,
			Total:  len(msgs),
			Unread: unread,
			Latest: &latest.MessageMetadata,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

// MailboxForAddress invokes policy.ParseMailboxName.
func (m *ManagerStub) MailboxForAddress(address string) (string, error) {
	addrPolicy := &policy.Addressing{Config: &config.Root{
		MailboxNaming: config.FullNaming,
	}}
	return addrPolicy.ExtractMailbox(address)
}

// MarkSeen marks a message as having been read.
func (m *ManagerStub) MarkSeen(mailbox, id string) error {
	if mailbox == "messageerr" {
		return errors.New("internal error")
	}
	for _, msg := range m.mailboxes[mailbox] {
		if msg.ID == id {
			msg.Seen = true
			return nil
		}
	}
	return storage.ErrNotExist
}
