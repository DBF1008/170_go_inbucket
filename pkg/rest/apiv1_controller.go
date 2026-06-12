package rest

import (
	"fmt"
	"io"
	"net/http"

	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/inbucket/inbucket/v3/pkg/message"
	"github.com/inbucket/inbucket/v3/pkg/rest/model"
	"github.com/inbucket/inbucket/v3/pkg/server/web"
	"github.com/inbucket/inbucket/v3/pkg/storage"
	"github.com/inbucket/inbucket/v3/pkg/stringutil"
)

// MailboxListV1 renders a list of messages in a mailbox. The optional seen, subject, address,
// start, and limit query parameters filter and paginate the results; the number of messages
// matching the filters (before pagination) is reported in the X-Total-Count response header.
func MailboxListV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	sel, err := parseMessageSelection(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	messages, err := ctx.Manager.GetMetadata(name)
	if err != nil {
		// This doesn't indicate empty, likely an IO error
		return fmt.Errorf("failed to get messages for %v: %v", name, err)
	}
	page, total := sel.Apply(messages)
	jmessages := make([]*model.JSONMessageHeaderV1, len(page))
	for i, msg := range page {
		jmessages[i] = &model.JSONMessageHeaderV1{
			Mailbox:     name,
			ID:          msg.ID,
			From:        stringutil.StringAddress(msg.From),
			To:          stringutil.StringAddressList(msg.To),
			Subject:     msg.Subject,
			Date:        msg.Date,
			PosixMillis: msg.Date.UnixNano() / 1000000,
			Size:        msg.Size,
			Seen:        msg.Seen,
		}
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	return web.RenderJSON(w, jmessages)
}

// parseMessageSelection builds a message.MessageSelection from the request's query parameters,
// returning an error if any parameter is malformed.
func parseMessageSelection(req *http.Request) (message.MessageSelection, error) {
	q := req.URL.Query()
	sel := message.MessageSelection{
		Subject: q.Get("subject"),
		Address: q.Get("address"),
	}
	if v := q.Get("start"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return sel, fmt.Errorf("invalid 'start' parameter: %q", v)
		}
		sel.Start = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return sel, fmt.Errorf("invalid 'limit' parameter: %q", v)
		}
		sel.Limit = n
	}
	if v := q.Get("seen"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return sel, fmt.Errorf("invalid 'seen' parameter: %q", v)
		}
		sel.Seen = &b
	}
	return sel, nil
}

// MailboxShowV1 renders a particular message from a mailbox
func MailboxShowV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	id := ctx.Vars["id"]
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	msg, err := ctx.Manager.GetMessage(name, id)
	if err != nil && err != storage.ErrNotExist {
		return fmt.Errorf("GetMessage(%q) failed: %v", id, err)
	}
	if msg == nil {
		http.NotFound(w, req)
		return nil
	}
	attachParts := msg.Attachments()
	attachments := make([]*model.JSONMessageAttachmentV1, len(attachParts))
	for i, part := range attachParts {
		content := part.Content
		// Example URL: http://localhost/serve/mailbox/swaks/0001/attach/0/favicon.png
		link := "http://" + req.Host + "/serve/mailbox/" + name + "/" + id + "/attach/" +
			strconv.Itoa(i) + "/" + part.FileName
		checksum := md5.Sum(content)
		attachments[i] = &model.JSONMessageAttachmentV1{
			ContentType:  part.ContentType,
			FileName:     part.FileName,
			DownloadLink: link,
			ViewLink:     link,
			MD5:          hex.EncodeToString(checksum[:]),
		}
	}
	return web.RenderJSON(w,
		&model.JSONMessageV1{
			Mailbox:     name,
			ID:          msg.ID,
			From:        stringutil.StringAddress(msg.From),
			To:          stringutil.StringAddressList(msg.To),
			Subject:     msg.Subject,
			Date:        msg.Date,
			PosixMillis: msg.Date.UnixNano() / 1000000,
			Size:        msg.Size,
			Seen:        msg.Seen,
			Header:      msg.Header(),
			Body: &model.JSONMessageBodyV1{
				Text: msg.Text(),
				HTML: msg.HTML(),
			},
			Attachments: attachments,
		})
}

// MailboxMarkSeenV1 marks a message as read.
func MailboxMarkSeenV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	id := ctx.Vars["id"]
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	dec := json.NewDecoder(req.Body)
	dm := model.JSONMessageHeaderV1{}
	if err := dec.Decode(&dm); err != nil {
		return fmt.Errorf("failed to decode JSON: %v", err)
	}
	if dm.Seen {
		err = ctx.Manager.MarkSeen(name, id)
		if err == storage.ErrNotExist {
			http.NotFound(w, req)
			return nil
		}
		if err != nil {
			// This doesn't indicate empty, likely an IO error
			return fmt.Errorf("MarkSeen(%q) failed: %v", id, err)
		}
	}
	return web.RenderJSON(w, "OK")
}

// MailboxPurgeV1 deletes all messages from a mailbox
func MailboxPurgeV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	// Delete all messages
	err = ctx.Manager.PurgeMessages(name)
	if err != nil {
		return fmt.Errorf("Mailbox(%q) purge failed: %v", name, err)
	}
	return web.RenderJSON(w, "OK")
}

// MailboxSourceV1 displays the raw source of a message, including headers. Renders text/plain
func MailboxSourceV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	id := ctx.Vars["id"]
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	r, err := ctx.Manager.SourceReader(name, id)
	if err != nil && err != storage.ErrNotExist {
		return fmt.Errorf("SourceReader(%q) failed: %v", id, err)
	}
	if r == nil {
		http.NotFound(w, req)
		return nil
	}
	// Output message source
	w.Header().Set("Content-Type", "text/plain")
	_, err = io.Copy(w, r)
	return err
}

// MailboxDeleteV1 removes a particular message from a mailbox
func MailboxDeleteV1(w http.ResponseWriter, req *http.Request, ctx *web.Context) (err error) {
	// Don't have to validate these aren't empty, Gorilla returns 404
	id := ctx.Vars["id"]
	name, err := ctx.Manager.MailboxForAddress(ctx.Vars["name"])
	if err != nil {
		return err
	}
	err = ctx.Manager.RemoveMessage(name, id)
	if err == storage.ErrNotExist {
		http.NotFound(w, req)
		return nil
	}
	if err != nil {
		// This doesn't indicate missing, likely an IO error
		return fmt.Errorf("RemoveMessage(%q) failed: %v", id, err)
	}
	return web.RenderJSON(w, "OK")
}
