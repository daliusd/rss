package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"os"
	"time"
)

// ====================================================================
// Email
// ====================================================================

const (
	mailTo   = "dalius.dobravolskas@gmail.com"
	smtpHost = "mail.ffff.lt"
	smtpPort = 587
)

var emailTmpl = template.Must(template.New("email").Funcs(template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>RSS Digest</title>
  <style>
    body { font-family: system-ui, sans-serif; background: #f5f5f5; color: #222; padding: 1rem; }
    h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
    h2 { font-size: 1.1rem; border-bottom: 2px solid #ddd; padding-bottom: 0.3rem; margin: 1.5rem 0 0.75rem; }
    .item { background: #fff; border: 1px solid #e0e0e0; border-radius: 6px; padding: 1rem; margin-bottom: 0.75rem; }
    .item h3 { font-size: 0.95rem; margin-bottom: 0.3rem; }
    .item h3 a { color: #0066cc; text-decoration: none; }
    .meta { font-size: 0.75rem; color: #888; margin-bottom: 0.5rem; }
    .content { font-size: 0.875rem; line-height: 1.6; }
    .content img { max-width: 100%; height: auto; }
  </style>
</head>
<body>
  <h1>RSS Digest — {{.Date}}</h1>
  <p>{{.Total}} new item(s) across {{len .Feeds}} feed(s).</p>
  {{range .Feeds}}
  <h2>{{.Name}}</h2>
  {{range .Items}}
  <div class="item">
    <h3><a href="{{.Link}}">{{.Title}}</a></h3>
    {{if not .Published.IsZero}}<p class="meta">{{.Published.Format "2 Jan 2006"}}</p>{{end}}
    {{if .Content}}<div class="content">{{safeHTML .Content}}</div>{{end}}
  </div>
  {{end}}
  {{end}}
</body>
</html>
`))

type emailFeedGroup struct {
	Name  string
	Items []RSSItem
}

type emailData struct {
	Date  string
	Total int
	Feeds []emailFeedGroup
}

func buildEmailHTML(newItems []RSSItem, now time.Time) (string, error) {
	// Group by feed, preserving order of first appearance.
	orderMap := map[string]int{}
	var groups []emailFeedGroup
	for _, item := range newItems {
		idx, ok := orderMap[item.FeedTitle]
		if !ok {
			idx = len(groups)
			orderMap[item.FeedTitle] = idx
			groups = append(groups, emailFeedGroup{Name: item.FeedTitle})
		}
		groups[idx].Items = append(groups[idx].Items, item)
	}

	data := emailData{
		Date:  now.UTC().Format("2 Jan 2006"),
		Total: len(newItems),
		Feeds: groups,
	}

	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render email template: %w", err)
	}
	return buf.String(), nil
}

func sendEmail(subject, htmlBody string) error {
	user := os.Getenv("MAIL_USER")
	pass := os.Getenv("MAIL_PASS")
	if user == "" || pass == "" {
		return fmt.Errorf("MAIL_USER or MAIL_PASS env variable is not set")
	}

	auth := smtp.PlainAuth("", user, pass, smtpHost)
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	// Build a minimal RFC 2822 + MIME message with HTML body.
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", user)
	fmt.Fprintf(&msg, "To: %s\r\n", mailTo)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=\"UTF-8\"\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	return smtp.SendMail(addr, auth, user, []string{mailTo}, msg.Bytes())
}

func logEmailError(err error) {
	log.Printf("[email] send error: %v", err)
}
