package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed dist/*.html
var htmlFS embed.FS

//go:embed dist/*.txt
var textFS embed.FS

var (
	htmlTemplates = template.Must(template.ParseFS(htmlFS, "dist/*.html"))
	textTemplates = texttemplate.Must(texttemplate.ParseFS(textFS, "dist/*.txt"))
)

// NotificationData holds the dynamic values injected into the email
// templates at runtime. The HTML template has pre-inlined CSS from the
// Maizzle build -- no runtime CSS processing occurs here.
type NotificationData struct {
	Email     string
	ID        string
	RequestID string
}

// RenderNotification renders the notification email templates with the
// given data and returns the HTML body and plaintext body.
func RenderNotification(data NotificationData) (string, string, error) {
	var htmlBuf, textBuf bytes.Buffer

	if err := htmlTemplates.ExecuteTemplate(&htmlBuf, "notification.html", data); err != nil {
		return "", "", fmt.Errorf("render html: %w", err)
	}
	if err := textTemplates.ExecuteTemplate(&textBuf, "notification.txt", data); err != nil {
		return "", "", fmt.Errorf("render text: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}
