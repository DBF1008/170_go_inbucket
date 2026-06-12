package web

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/inbucket/inbucket/v3/pkg/stringutil"
)

// From http://daringfireball.net/2010/07/improved_regex_for_matching_urls
var urlRE = regexp.MustCompile("(?i)\\b((?:[a-z][\\w-]+:(?:/{1,3}|[a-z0-9%])|www\\d{0,3}[.]|[a-z0-9.\\-]+[.][a-z]{2,4}/)(?:[^\\s()<>]+|\\(([^\\s()<>]+|(\\([^\\s()<>]+\\)))*\\))+(?:\\(([^\\s()<>]+|(\\([^\\s()<>]+\\)))*\\)|[^\\s`!()\\[\\]{};:'\".,<>?«»“”‘’]))")

// TextToHTML takes plain text, escapes it and tries to pretty it up for
// HTML display
func TextToHTML(text string) string {
	text = html.EscapeString(text)
	text = urlRE.ReplaceAllStringFunc(text, WrapURL)
	replacer := strings.NewReplacer("\r\n", "<br/>\n", "\r", "<br/>\n", "\n", "<br/>\n")
	return replacer.Replace(text)
}

// WrapURL wraps a <a href> tag around the provided URL
func WrapURL(url string) string {
	unescaped := strings.ReplaceAll(url, "&amp;", "&")
	return fmt.Sprintf("<a href=\"%s\" target=\"_blank\">%s</a>", unescaped, url)
}

// AttachmentURL builds a fully-qualified URL for an attachment endpoint. It
// resolves the scheme and host by consulting X-Forwarded-Proto and
// X-Forwarded-Host headers (set by HTTPS-terminating reverse proxies), falling
// back to the original request. The configured BasePath prefix is applied so the
// link works when the service is mounted under a sub-path.
func AttachmentURL(req *http.Request, basePath string, name, id string, index int, fileName string) string {
	// Scheme: prefer X-Forwarded-Proto (e.g. "https" when behind TLS-terminating proxy).
	scheme := "http"
	if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
		// X-Forwarded-Proto may contain multiple values ("https, http"); take the first.
		if i := strings.IndexByte(proto, ','); i != -1 {
			proto = strings.TrimSpace(proto[:i])
		}
		scheme = strings.ToLower(proto)
	}

	// Host: prefer X-Forwarded-Host (the external host the client used).
	host := req.Host
	if fh := req.Header.Get("X-Forwarded-Host"); fh != "" {
		// May contain multiple values; take the first.
		if i := strings.IndexByte(fh, ','); i != -1 {
			fh = strings.TrimSpace(fh[:i])
		}
		host = fh
	}

	prefix := stringutil.MakePathPrefixer(basePath)
	path := prefix("/serve/mailbox/" + name + "/" + id + "/attach/" + fmt.Sprint(index) + "/" + url.PathEscape(fileName))
	return scheme + "://" + host + path
}
