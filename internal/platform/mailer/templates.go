package mailer

import (
	"bytes"
	"fmt"
	"html/template"
)

var invitationHTML = template.Must(template.New("invite").Parse(`<!doctype html>
<html><body style="font-family:Arial,sans-serif">
<p>Hi {{.Name}},</p>
<p>You have been invited to join <strong>{{.TenantName}}</strong>.</p>
<p><a href="{{.Link}}">Verify your email and set your password</a></p>
<p>This link expires in {{.ExpiresIn}}.</p>
<p>If you did not expect this invitation you can ignore this email.</p>
</body></html>`))

type InvitationData struct {
	Name       string
	TenantName string
	Link       string
	ExpiresIn  string
}

func InvitationEmail(to string, d InvitationData) (Message, error) {
	var html bytes.Buffer
	if err := invitationHTML.Execute(&html, d); err != nil {
		return Message{}, err
	}
	text := fmt.Sprintf("Hi %s,\n\nYou have been invited to join %s.\n\nVerify your email and set your password:\n%s\n\nThis link expires in %s.\n",
		d.Name, d.TenantName, d.Link, d.ExpiresIn)
	return Message{
		To:       to,
		Subject:  fmt.Sprintf("You're invited to %s", d.TenantName),
		TextBody: text,
		HTMLBody: html.String(),
	}, nil
}
