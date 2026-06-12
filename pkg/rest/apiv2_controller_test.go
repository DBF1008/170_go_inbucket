package rest

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/inbucket/inbucket/v3/pkg/extension/event"
	"github.com/inbucket/inbucket/v3/pkg/message"
	"github.com/inbucket/inbucket/v3/pkg/test"
	"github.com/jhillyerd/enmime/v2"
)

// TestRestV2MailboxFlow exercises the normal v2 lifecycle within a single mailbox: list, show,
// source, mark-seen, delete and purge. It also verifies that mailboxes are isolated from one
// another (single-mailbox scenario).
func TestRestV2MailboxFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPDT := time.FixedZone("PDT", -7*3600)
	tzPST := time.FixedZone("PST", -8*3600)

	// 0001 carries a full envelope so it can be shown (body, header, attachments).
	msg1 := message.New(
		event.MessageMetadata{
			Mailbox: "good",
			ID:      "0001",
			From:    &mail.Address{Name: "", Address: "from1@host"},
			To:      []*mail.Address{{Name: "", Address: "to1@host"}},
			Subject: "subject 1",
			Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
		},
		&enmime.Envelope{
			Text: "This is some text",
			HTML: "This is some HTML",
			Root: &enmime.Part{
				Header: textproto.MIMEHeader{
					"From": []string{"noreply@inbucket.org"},
				},
			},
		},
	)
	meta2 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0002",
		From:    &mail.Address{Name: "", Address: "from2@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 2",
		Date:    time.Date(2012, 7, 1, 10, 11, 12, 253, tzPDT),
	}
	mm.AddMessage("good", msg1)
	mm.AddMessage("good", &message.Message{MessageMetadata: meta2})

	// List: both messages, scoped to the requested mailbox, seen defaults to false.
	result := getListV2(t, "good")
	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %v", len(result))
	}
	decodedStringEquals(t, result, "[0]/mailbox", "good")
	decodedStringEquals(t, result, "[0]/id", "0001")
	decodedBoolEquals(t, result, "[0]/seen", false)
	decodedStringEquals(t, result, "[1]/id", "0002")

	// A different mailbox is independent (single-mailbox isolation).
	if other := getListV2(t, "empty"); len(other) != 0 {
		t.Fatalf("Expected empty mailbox to have 0 results, got %v", len(other))
	}

	// Show a single message including its body and headers.
	w, err := testRestGet("http://localhost/api/v2/mailbox/good/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("show: expected code 200, got %v", w.Code)
	}
	var shown map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&shown); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}
	decodedStringEquals(t, shown, "id", "0001")
	decodedStringEquals(t, shown, "subject", "subject 1")
	decodedStringEquals(t, shown, "body/text", "This is some text")
	decodedStringEquals(t, shown, "header/From/[0]", "noreply@inbucket.org")

	// Read the raw source as text/plain.
	w, err = testRestGet("http://localhost/api/v2/mailbox/good/0001/source")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("source: expected code 200, got %v", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("source: expected text/plain content-type, got %q", ct)
	}
	if body := w.Body.String(); body != "Subject: subject 1\r\n" {
		t.Errorf("source: unexpected body %q", body)
	}

	// Mark 0001 as seen, leaving 0002 unread.
	w, err = testRestPatch("http://localhost/api/v2/mailbox/good/0001", `{"seen":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("mark-seen: expected code 200, got %v", w.Code)
	}
	result = getListV2(t, "good")
	decodedStringEquals(t, result, "[0]/id", "0001")
	decodedBoolEquals(t, result, "[0]/seen", true)
	decodedStringEquals(t, result, "[1]/id", "0002")
	decodedBoolEquals(t, result, "[1]/seen", false)

	// Delete a single message.
	w, err = testRestDelete("http://localhost/api/v2/mailbox/good/0002")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("delete: expected code 200, got %v", w.Code)
	}
	result = getListV2(t, "good")
	if len(result) != 1 {
		t.Fatalf("after delete: expected 1 result, got %v", len(result))
	}
	decodedStringEquals(t, result, "[0]/id", "0001")

	// Purge the whole mailbox.
	w, err = testRestDelete("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("purge: expected code 200, got %v", w.Code)
	}
	if result = getListV2(t, "good"); len(result) != 0 {
		t.Fatalf("after purge: expected 0 results, got %v", len(result))
	}

	if t.Failed() {
		// Wait for handler to finish logging, then dump buffered log data.
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestRestV2MessageNotFound verifies that every per-message v2 operation returns 404 for a message
// that does not exist.
func TestRestV2MessageNotFound(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	cases := []struct {
		name string
		do   func() (*httptest.ResponseRecorder, error)
	}{
		{"show", func() (*httptest.ResponseRecorder, error) {
			return testRestGet("http://localhost/api/v2/mailbox/empty/0001")
		}},
		{"source", func() (*httptest.ResponseRecorder, error) {
			return testRestGet("http://localhost/api/v2/mailbox/empty/0001/source")
		}},
		{"mark-seen", func() (*httptest.ResponseRecorder, error) {
			return testRestPatch("http://localhost/api/v2/mailbox/empty/0001", `{"seen":true}`)
		}},
		{"delete", func() (*httptest.ResponseRecorder, error) {
			return testRestDelete("http://localhost/api/v2/mailbox/empty/0001")
		}},
	}
	for _, tc := range cases {
		w, err := tc.do()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if w.Code != 404 {
			t.Errorf("%s: expected code 404, got %v", tc.name, w.Code)
		}
	}

	if t.Failed() {
		// Wait for handler to finish logging, then dump buffered log data.
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// getListV2 fetches and decodes the v2 message list for a mailbox.
func getListV2(t *testing.T, mailbox string) []interface{} {
	t.Helper()
	w, err := testRestGet("http://localhost/api/v2/mailbox/" + mailbox)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("list %s: expected code 200, got %v", mailbox, w.Code)
	}
	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("list %s: failed to decode JSON: %v", mailbox, err)
	}
	return result
}
