package web

import (
	"net/http"
	"testing"
)

func TestTextToHtml(t *testing.T) {
	testCases := []struct {
		input, want string
	}{
		{
			input: "html",
			want:  "html",
		},
		// Check it escapes.
		{
			input: "<html>",
			want:  "&lt;html&gt;",
		},
		// Check for linebreaks.
		{
			input: "line\nbreak",
			want:  "line<br/>\nbreak",
		},
		{
			input: "line\r\nbreak",
			want:  "line<br/>\nbreak",
		},
		{
			input: "line\rbreak",
			want:  "line<br/>\nbreak",
		},
		// Check URL detection.
		{
			input: "http://google.com/",
			want:  "<a href=\"http://google.com/\" target=\"_blank\">http://google.com/</a>",
		},
		{
			input: "http://a.com/?q=a&n=v",
			want:  "<a href=\"http://a.com/?q=a&n=v\" target=\"_blank\">http://a.com/?q=a&amp;n=v</a>",
		},
		{
			input: "(http://a.com/?q=a&n=v)",
			want:  "(<a href=\"http://a.com/?q=a&n=v\" target=\"_blank\">http://a.com/?q=a&amp;n=v</a>)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			got := TextToHTML(tc.input)
			if got != tc.want {
				t.Errorf("TextToHTML(%q)\ngot : %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAttachmentURL(t *testing.T) {
	tests := []struct {
		name     string
		reqURL   string
		host     string
		headers  map[string]string
		basePath string
		mbName   string
		msgID    string
		index    int
		fileName string
		want     string
	}{
		{
			name:     "direct HTTP no base path",
			reqURL:   "http://localhost:9000/api/v1/mailbox/user/0001",
			host:     "localhost:9000",
			basePath: "",
			mbName:   "user",
			msgID:    "0001",
			index:    0,
			fileName: "report.pdf",
			want:     "http://localhost:9000/serve/mailbox/user/0001/attach/0/report.pdf",
		},
		{
			name:     "direct HTTP with base path",
			reqURL:   "http://example.com:9000/inbucket/api/v1/mailbox/user/0001",
			host:     "example.com:9000",
			basePath: "inbucket",
			mbName:   "user",
			msgID:    "0001",
			index:    0,
			fileName: "report.pdf",
			want:     "http://example.com:9000/inbucket/serve/mailbox/user/0001/attach/0/report.pdf",
		},
		{
			name:   "HTTPS reverse proxy via X-Forwarded-Proto",
			reqURL: "http://internal:9000/api/v1/mailbox/user/0001",
			host:   "internal:9000",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "mail.example.com",
			},
			basePath: "",
			mbName:   "user",
			msgID:    "0001",
			index:    2,
			fileName: "image.png",
			want:     "https://mail.example.com/serve/mailbox/user/0001/attach/2/image.png",
		},
		{
			name:   "HTTPS proxy with base path",
			reqURL: "http://internal:9000/inbucket/api/v1/mailbox/user/0001",
			host:   "internal:9000",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "mail.example.com",
			},
			basePath: "/inbucket",
			mbName:   "user",
			msgID:    "0001",
			index:    0,
			fileName: "data.csv",
			want:     "https://mail.example.com/inbucket/serve/mailbox/user/0001/attach/0/data.csv",
		},
		{
			name:   "X-Forwarded-Proto with multiple values",
			reqURL: "http://internal:9000/api/v1/mailbox/user/0001",
			host:   "internal:9000",
			headers: map[string]string{
				"X-Forwarded-Proto": "https, http",
				"X-Forwarded-Host":  "proxy.example.com, origin.example.com",
			},
			basePath: "",
			mbName:   "user",
			msgID:    "0001",
			index:    0,
			fileName: "file.txt",
			want:     "https://proxy.example.com/serve/mailbox/user/0001/attach/0/file.txt",
		},
		{
			name:     "file name with special characters",
			reqURL:   "http://localhost:9000/api/v1/mailbox/user/0001",
			host:     "localhost:9000",
			basePath: "",
			mbName:   "user",
			msgID:    "0001",
			index:    0,
			fileName: "my file (1).pdf",
			want:     "http://localhost:9000/serve/mailbox/user/0001/attach/0/my%20file%20%281%29.pdf",
		},
		{
			name:   "base path with leading slash stripped",
			reqURL: "http://localhost:9000/api/v1/mailbox/user/0001",
			host:   "localhost:9000",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
			},
			basePath: "/sub/path",
			mbName:   "user",
			msgID:    "0001",
			index:    1,
			fileName: "attachment.bin",
			want:     "https://localhost:9000/sub/path/serve/mailbox/user/0001/attach/1/attachment.bin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tc.reqURL, nil)
			req.Host = tc.host
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			got := AttachmentURL(req, tc.basePath, tc.mbName, tc.msgID, tc.index, tc.fileName)
			if got != tc.want {
				t.Errorf("AttachmentURL()\n got:  %q\n want: %q", got, tc.want)
			}
		})
	}
}
