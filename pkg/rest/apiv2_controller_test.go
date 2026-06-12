package rest

import (
	"encoding/json"
	"io"
	"net/mail"
	"net/textproto"
	"os"
	"testing"
	"time"

	"github.com/inbucket/inbucket/v3/pkg/extension/event"
	"github.com/inbucket/inbucket/v3/pkg/message"
	"github.com/inbucket/inbucket/v3/pkg/test"
	"github.com/jhillyerd/enmime/v2"
)

// TestV2MailboxList_NormalFlow tests listing messages in a mailbox with multiple messages.
func TestV2MailboxList_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPDT := time.FixedZone("PDT", -7*3600)
	tzPST := time.FixedZone("PST", -8*3600)
	meta1 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from1@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 1",
		Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
	}
	meta2 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0002",
		From:    &mail.Address{Name: "", Address: "from2@host"},
		To:      []*mail.Address{{Name: "", Address: "to2@host"}, {Name: "", Address: "to3@host"}},
		Subject: "subject 2",
		Date:    time.Date(2012, 7, 1, 10, 11, 12, 253, tzPDT),
		Seen:    true,
	}
	mm.AddMessage("good", &message.Message{MessageMetadata: meta1})
	mm.AddMessage("good", &message.Message{MessageMetadata: meta2})

	w, err := testRestGet("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %v", len(result))
	}

	// Verify first message
	decodedStringEquals(t, result, "[0]/mailbox", "good")
	decodedStringEquals(t, result, "[0]/id", "0001")
	decodedStringEquals(t, result, "[0]/from", "<from1@host>")
	decodedStringEquals(t, result, "[0]/to/[0]", "<to1@host>")
	decodedStringEquals(t, result, "[0]/subject", "subject 1")
	decodedStringEquals(t, result, "[0]/date", "2012-02-01T10:11:12.000000253-08:00")
	decodedNumberEquals(t, result, "[0]/posix-millis", 1328119872000)
	decodedNumberEquals(t, result, "[0]/size", 0)
	decodedBoolEquals(t, result, "[0]/seen", false)

	// Verify second message
	decodedStringEquals(t, result, "[1]/mailbox", "good")
	decodedStringEquals(t, result, "[1]/id", "0002")
	decodedStringEquals(t, result, "[1]/from", "<from2@host>")
	decodedStringEquals(t, result, "[1]/to/[0]", "<to2@host>")
	decodedStringEquals(t, result, "[1]/to/[1]", "<to3@host>")
	decodedStringEquals(t, result, "[1]/subject", "subject 2")
	decodedBoolEquals(t, result, "[1]/seen", true)

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MailboxList_EmptyMailbox tests listing messages in an empty mailbox.
func TestV2MailboxList_EmptyMailbox(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/empty")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Errorf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 results, got %v", len(result))
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MailboxList_InvalidMailbox tests listing messages with an invalid mailbox name.
func TestV2MailboxList_InvalidMailbox(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/foo%20bar")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 {
		t.Errorf("Expected code 500, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MailboxList_InternalError tests listing messages when manager returns an error.
func TestV2MailboxList_InternalError(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/messageserr")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 {
		t.Errorf("Expected code 500, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MailboxList_SingleMailbox tests listing messages in a mailbox with a single message.
func TestV2MailboxList_SingleMailbox(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta := event.MessageMetadata{
		Mailbox: "solo",
		ID:      "0001",
		From:    &mail.Address{Name: "Alice", Address: "alice@host"},
		To:      []*mail.Address{{Name: "Bob", Address: "bob@host"}},
		Subject: "only message",
		Date:    time.Date(2024, 1, 15, 9, 30, 0, 0, tzPST),
		Size:    1234,
	}
	mm.AddMessage("solo", &message.Message{MessageMetadata: meta})

	w, err := testRestGet("http://localhost/api/v2/mailbox/solo")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %v", len(result))
	}

	decodedStringEquals(t, result, "[0]/mailbox", "solo")
	decodedStringEquals(t, result, "[0]/id", "0001")
	decodedStringEquals(t, result, "[0]/from", "Alice <alice@host>")
	decodedStringEquals(t, result, "[0]/to/[0]", "Bob <bob@host>")
	decodedStringEquals(t, result, "[0]/subject", "only message")
	decodedNumberEquals(t, result, "[0]/size", 1234)
	decodedBoolEquals(t, result, "[0]/seen", false)

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Message_NormalFlow tests getting a message with body, headers, and attachments.
func TestV2Message_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	msg := message.New(
		event.MessageMetadata{
			Mailbox: "good",
			ID:      "0001",
			From:    &mail.Address{Name: "", Address: "from1@host"},
			To:      []*mail.Address{{Name: "", Address: "to1@host"}},
			Subject: "subject 1",
			Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
			Seen:    true,
		},
		&enmime.Envelope{
			Text: "This is some text",
			HTML: "This is some HTML",
			Root: &enmime.Part{
				Header: textproto.MIMEHeader{
					"To":   []string{"fred@fish.com", "keyword@nsa.gov"},
					"From": []string{"noreply@inbucket.org"},
				},
			},
			Attachments: []*enmime.Part{{
				FileName:    "favicon.png",
				ContentType: "image/png",
			}},
			Inlines: []*enmime.Part{{
				FileName:    "statement.pdf",
				ContentType: "application/pdf",
			}},
		},
	)
	mm.AddMessage("good", msg)

	w, err := testRestGet("http://localhost/api/v2/mailbox/good/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result map[string]interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	decodedStringEquals(t, result, "mailbox", "good")
	decodedStringEquals(t, result, "id", "0001")
	decodedStringEquals(t, result, "from", "<from1@host>")
	decodedStringEquals(t, result, "to/[0]", "<to1@host>")
	decodedStringEquals(t, result, "subject", "subject 1")
	decodedNumberEquals(t, result, "posix-millis", 1328119872000)
	decodedBoolEquals(t, result, "seen", true)
	decodedStringEquals(t, result, "body/text", "This is some text")
	decodedStringEquals(t, result, "body/html", "This is some HTML")
	decodedStringEquals(t, result, "header/To/[0]", "fred@fish.com")
	decodedStringEquals(t, result, "header/To/[1]", "keyword@nsa.gov")
	decodedStringEquals(t, result, "header/From/[0]", "noreply@inbucket.org")
	decodedStringEquals(t, result, "attachments/[0]/filename", "statement.pdf")
	decodedStringEquals(t, result, "attachments/[0]/content-type", "application/pdf")
	decodedStringEquals(t, result, "attachments/[0]/download-link",
		"http://localhost/serve/mailbox/good/0001/attach/0/statement.pdf")
	decodedStringEquals(t, result, "attachments/[1]/filename", "favicon.png")
	decodedStringEquals(t, result, "attachments/[1]/content-type", "image/png")

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Message_NotExist tests getting a message that does not exist returns 404.
func TestV2Message_NotExist(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/empty/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 {
		t.Errorf("Expected code 404, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Message_InvalidMailbox tests getting a message from an invalid mailbox.
func TestV2Message_InvalidMailbox(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/foo%20bar/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 {
		t.Errorf("Expected code 500, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Message_InternalError tests getting a message when manager returns an error.
func TestV2Message_InternalError(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/messageerr/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 {
		t.Errorf("Expected code 500, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MarkSeen_NormalFlow tests marking a message as seen.
func TestV2MarkSeen_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta1 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from1@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 1",
		Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
	}
	meta2 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0002",
		From:    &mail.Address{Name: "", Address: "from2@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 2",
		Date:    time.Date(2012, 7, 1, 10, 11, 12, 253, tzPST),
	}
	mm.AddMessage("good", &message.Message{MessageMetadata: meta1})
	mm.AddMessage("good", &message.Message{MessageMetadata: meta2})

	// Mark message 0002 as seen
	w, err := testRestPatch("http://localhost/api/v2/mailbox/good/0002", `{"seen":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	// Verify via v2 list
	w, err = testRestGet("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %v", len(result))
	}

	decodedStringEquals(t, result, "[0]/id", "0001")
	decodedBoolEquals(t, result, "[0]/seen", false)
	decodedStringEquals(t, result, "[1]/id", "0002")
	decodedBoolEquals(t, result, "[1]/seen", true)

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2MarkSeen_NotExist tests marking a non-existent message returns 404.
func TestV2MarkSeen_NotExist(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestPatch("http://localhost/api/v2/mailbox/empty/9999", `{"seen":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 {
		t.Errorf("Expected code 404, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Delete_NormalFlow tests deleting a specific message.
func TestV2Delete_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta1 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from1@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 1",
		Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
	}
	meta2 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0002",
		From:    &mail.Address{Name: "", Address: "from2@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 2",
		Date:    time.Date(2012, 7, 1, 10, 11, 12, 253, tzPST),
	}
	mm.AddMessage("good", &message.Message{MessageMetadata: meta1})
	mm.AddMessage("good", &message.Message{MessageMetadata: meta2})

	// Delete message 0001
	w, err := testRestDelete("http://localhost/api/v2/mailbox/good/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	// Verify only 0002 remains via v2 list
	w, err = testRestGet("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %v", len(result))
	}
	decodedStringEquals(t, result, "[0]/id", "0002")

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Delete_NotExist tests deleting a non-existent message returns 404.
func TestV2Delete_NotExist(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestDelete("http://localhost/api/v2/mailbox/empty/9999")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 {
		t.Errorf("Expected code 404, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Purge_NormalFlow tests purging all messages from a mailbox.
func TestV2Purge_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta1 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from1@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 1",
		Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
	}
	meta2 := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0002",
		From:    &mail.Address{Name: "", Address: "from2@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 2",
		Date:    time.Date(2012, 7, 1, 10, 11, 12, 253, tzPST),
	}
	mm.AddMessage("good", &message.Message{MessageMetadata: meta1})
	mm.AddMessage("good", &message.Message{MessageMetadata: meta2})

	// Purge mailbox
	w, err := testRestDelete("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	// Verify mailbox is empty via v2 list
	w, err = testRestGet("http://localhost/api/v2/mailbox/good")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 results, got %v", len(result))
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Source_NormalFlow tests reading raw message source.
func TestV2Source_NormalFlow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta := event.MessageMetadata{
		Mailbox: "good",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from1@host"},
		To:      []*mail.Address{{Name: "", Address: "to1@host"}},
		Subject: "subject 1",
		Date:    time.Date(2012, 2, 1, 10, 11, 12, 253, tzPST),
	}
	mm.AddMessage("good", &message.Message{MessageMetadata: meta})

	rawSource := "From: from1@host\r\nTo: to1@host\r\nSubject: subject 1\r\n\r\nHello World"
	mm.AddSource("good", "0001", rawSource)

	w, err := testRestGet("http://localhost/api/v2/mailbox/good/0001/source")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("Expected Content-Type text/plain, got %v", ct)
	}

	body := w.Body.String()
	if body != rawSource {
		t.Errorf("Expected body %q, got %q", rawSource, body)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Source_NotExist tests reading raw source of a non-existent message returns 404.
func TestV2Source_NotExist(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	w, err := testRestGet("http://localhost/api/v2/mailbox/empty/9999/source")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 {
		t.Errorf("Expected code 404, got %v", w.Code)
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2FullWorkflow tests a complete workflow: list, show, mark seen, delete, verify.
func TestV2FullWorkflow(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta1 := event.MessageMetadata{
		Mailbox: "workflow",
		ID:      "0001",
		From:    &mail.Address{Name: "Sender", Address: "sender@test.com"},
		To:      []*mail.Address{{Name: "Receiver", Address: "receiver@test.com"}},
		Subject: "test workflow",
		Date:    time.Date(2024, 6, 15, 14, 30, 0, 0, tzPST),
		Size:    512,
	}
	msg1 := message.New(
		meta1,
		&enmime.Envelope{
			Text: "Workflow test body",
			Root: &enmime.Part{
				Header: textproto.MIMEHeader{
					"From":    []string{"sender@test.com"},
					"To":      []string{"receiver@test.com"},
					"Subject": []string{"test workflow"},
				},
			},
		},
	)
	mm.AddMessage("workflow", msg1)
	mm.AddSource("workflow", "0001", "From: sender@test.com\r\nSubject: test workflow\r\n\r\nWorkflow test body")

	// Step 1: List messages - should have 1
	w, err := testRestGet("http://localhost/api/v2/mailbox/workflow")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("List: expected code 200, got %v", w.Code)
	}
	dec := json.NewDecoder(w.Body)
	var listResult []interface{}
	if err := dec.Decode(&listResult); err != nil {
		t.Fatalf("List: failed to decode JSON: %v", err)
	}
	if len(listResult) != 1 {
		t.Fatalf("List: expected 1 result, got %v", len(listResult))
	}
	decodedBoolEquals(t, listResult, "[0]/seen", false)

	// Step 2: Show message detail
	w, err = testRestGet("http://localhost/api/v2/mailbox/workflow/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Show: expected code 200, got %v", w.Code)
	}
	dec = json.NewDecoder(w.Body)
	var msgResult map[string]interface{}
	if err := dec.Decode(&msgResult); err != nil {
		t.Fatalf("Show: failed to decode JSON: %v", err)
	}
	decodedStringEquals(t, msgResult, "id", "0001")
	decodedStringEquals(t, msgResult, "subject", "test workflow")
	decodedStringEquals(t, msgResult, "body/text", "Workflow test body")

	// Step 3: Read source
	w, err = testRestGet("http://localhost/api/v2/mailbox/workflow/0001/source")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Source: expected code 200, got %v", w.Code)
	}
	if w.Body.String() == "" {
		t.Error("Source: expected non-empty body")
	}

	// Step 4: Mark as seen
	w, err = testRestPatch("http://localhost/api/v2/mailbox/workflow/0001", `{"seen":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("MarkSeen: expected code 200, got %v", w.Code)
	}

	// Verify seen status
	w, err = testRestGet("http://localhost/api/v2/mailbox/workflow")
	if err != nil {
		t.Fatal(err)
	}
	dec = json.NewDecoder(w.Body)
	var listResult2 []interface{}
	if err := dec.Decode(&listResult2); err != nil {
		t.Fatalf("Verify seen: failed to decode JSON: %v", err)
	}
	decodedBoolEquals(t, listResult2, "[0]/seen", true)

	// Step 5: Delete message
	w, err = testRestDelete("http://localhost/api/v2/mailbox/workflow/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Delete: expected code 200, got %v", w.Code)
	}

	// Step 6: Verify message is gone (404)
	w, err = testRestGet("http://localhost/api/v2/mailbox/workflow/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 {
		t.Errorf("Verify deleted: expected code 404, got %v", w.Code)
	}

	// Step 7: Verify mailbox is empty
	w, err = testRestGet("http://localhost/api/v2/mailbox/workflow")
	if err != nil {
		t.Fatal(err)
	}
	dec = json.NewDecoder(w.Body)
	var listResult3 []interface{}
	if err := dec.Decode(&listResult3); err != nil {
		t.Fatalf("Verify empty: failed to decode JSON: %v", err)
	}
	if len(listResult3) != 0 {
		t.Errorf("Verify empty: expected 0 results, got %v", len(listResult3))
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}

// TestV2Delete_SingleMailbox tests deleting the only message in a mailbox.
func TestV2Delete_SingleMailbox(t *testing.T) {
	mm := test.NewManager()
	logbuf := setupWebServer(mm)

	tzPST := time.FixedZone("PST", -8*3600)
	meta := event.MessageMetadata{
		Mailbox: "single",
		ID:      "0001",
		From:    &mail.Address{Name: "", Address: "from@host"},
		To:      []*mail.Address{{Name: "", Address: "to@host"}},
		Subject: "only one",
		Date:    time.Date(2024, 3, 1, 12, 0, 0, 0, tzPST),
	}
	mm.AddMessage("single", &message.Message{MessageMetadata: meta})

	// Delete the only message
	w, err := testRestDelete("http://localhost/api/v2/mailbox/single/0001")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	// Verify mailbox is now empty
	w, err = testRestGet("http://localhost/api/v2/mailbox/single")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("Expected code 200, got %v", w.Code)
	}

	dec := json.NewDecoder(w.Body)
	var result []interface{}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 results after delete, got %v", len(result))
	}

	if t.Failed() {
		time.Sleep(2 * time.Second)
		_, _ = io.Copy(os.Stderr, logbuf)
	}
}
