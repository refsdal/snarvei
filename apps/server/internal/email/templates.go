package email

import (
	"fmt"
	"html"
)

// Template is a rendered mail without a recipient.
type Template struct {
	Subject string
	Text    string
	HTML    string
}

// To binds the template to a recipient.
func (t Template) To(addr string) Message {
	return Message{To: addr, Subject: t.Subject, Text: t.Text, HTML: t.HTML}
}

func layout(appName, title, bodyHTML string) string {
	return fmt.Sprintf(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;line-height:1.5;color:#111">
<h2 style="margin:0 0 16px">%s</h2>
%s
<p style="color:#666;font-size:12px;margin-top:32px">Sent by %s. If you did not expect this email you can ignore it.</p>
</body></html>`, html.EscapeString(title), bodyHTML, html.EscapeString(appName))
}

func linkButton(href, label string) string {
	h := html.EscapeString(href)
	return fmt.Sprintf(`<p><a href="%s" style="display:inline-block;padding:10px 16px;background:#4f46e5;color:#fff;text-decoration:none;border-radius:6px">%s</a></p><p style="font-size:12px;color:#666">Or open this link: %s</p>`, h, html.EscapeString(label), h)
}

// Invitation is the organization invitation mail; inviterName may be empty.
func Invitation(appName, orgName, inviterName, link string) Template {
	by := ""
	if inviterName != "" {
		by = " by " + inviterName
	}
	return Template{
		Subject: fmt.Sprintf("You have been invited to %s on %s", orgName, appName),
		Text:    fmt.Sprintf("You have been invited%s to join %s on %s.\n\nAccept the invitation: %s\n\nIf you did not expect this invitation you can ignore this email.", by, orgName, appName, link),
		HTML: layout(appName, "Join "+orgName,
			fmt.Sprintf("<p>You have been invited%s to join <strong>%s</strong> on %s.</p>%s", html.EscapeString(by), html.EscapeString(orgName), html.EscapeString(appName), linkButton(link, "Accept invitation"))),
	}
}

// PasswordReset is the forgot-password mail.
func PasswordReset(appName, link string) Template {
	return Template{
		Subject: fmt.Sprintf("Reset your %s password", appName),
		Text:    fmt.Sprintf("Reset your %s password: %s\n\nIf you did not request this, ignore this email.", appName, link),
		HTML:    layout(appName, "Reset your password", "<p>Use the button below to choose a new password.</p>"+linkButton(link, "Reset password")),
	}
}

// EmailChange confirms a new address.
func EmailChange(appName, newEmail, link string) Template {
	return Template{
		Subject: fmt.Sprintf("Confirm your new %s email address", appName),
		Text:    fmt.Sprintf("Confirm changing your %s email address to %s: %s", appName, newEmail, link),
		HTML: layout(appName, "Confirm your new email address",
			fmt.Sprintf("<p>Confirm changing your email address to <strong>%s</strong>.</p>%s", html.EscapeString(newEmail), linkButton(link, "Confirm change"))),
	}
}
