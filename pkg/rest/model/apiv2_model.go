package model

import (
	"time"
)

// JSONMessageIDV2 uniquely identifies a message.
type JSONMessageIDV2 struct {
	Mailbox string `json:"mailbox"`
	ID      string `json:"id"`
}

// JSONMonitorEventV2 contains events for the Inbucket mailbox and monitor tabs.
type JSONMonitorEventV2 struct {
	// Event variant: `message-deleted`, `message-stored`.
	Variant    string               `json:"variant"`
	Identifier *JSONMessageIDV2     `json:"identifier"`
	Header     *JSONMessageHeaderV1 `json:"header"`
}

// JSONMessageHeaderV2 contains the basic header data for a message (v2).
type JSONMessageHeaderV2 struct {
	Mailbox     string    `json:"mailbox"`
	ID          string    `json:"id"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	Date        time.Time `json:"date"`
	PosixMillis int64     `json:"posix-millis"`
	Size        int64     `json:"size"`
	Seen        bool      `json:"seen"`
}

// JSONMessageV2 contains the full message data including body, headers, and attachments (v2).
type JSONMessageV2 struct {
	Mailbox     string                     `json:"mailbox"`
	ID          string                     `json:"id"`
	From        string                     `json:"from"`
	To          []string                   `json:"to"`
	Subject     string                     `json:"subject"`
	Date        time.Time                  `json:"date"`
	PosixMillis int64                      `json:"posix-millis"`
	Size        int64                      `json:"size"`
	Seen        bool                       `json:"seen"`
	Body        *JSONMessageBodyV2         `json:"body"`
	Header      map[string][]string        `json:"header"`
	Attachments []*JSONMessageAttachmentV2 `json:"attachments"`
}

// JSONMessageBodyV2 contains the Text and HTML versions of the message body (v2).
type JSONMessageBodyV2 struct {
	Text string `json:"text"`
	HTML string `json:"html"`
}

// JSONMessageAttachmentV2 contains information about a MIME attachment (v2).
type JSONMessageAttachmentV2 struct {
	FileName     string `json:"filename"`
	ContentType  string `json:"content-type"`
	DownloadLink string `json:"download-link"`
	ViewLink     string `json:"view-link"`
	MD5          string `json:"md5"`
}
