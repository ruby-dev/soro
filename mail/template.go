package mail

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"io"
	texttemplate "text/template"
)

type Templates struct {
	subject *texttemplate.Template
	text    *texttemplate.Template
	html    *htmltemplate.Template
}

func ParseTemplates(name, subject, textBody, htmlBody string) (*Templates, error) {
	if name == "" || subject == "" {
		return nil, fmt.Errorf("mail template name and subject are required")
	}
	subjectTemplate, err := texttemplate.New(name + "-subject").Option("missingkey=error").Parse(subject)
	if err != nil {
		return nil, fmt.Errorf("mail subject template: %w", err)
	}
	templates := &Templates{subject: subjectTemplate}
	if textBody != "" {
		templates.text, err = texttemplate.New(name + "-text").Option("missingkey=error").Parse(textBody)
		if err != nil {
			return nil, fmt.Errorf("mail text template: %w", err)
		}
	}
	if htmlBody != "" {
		templates.html, err = htmltemplate.New(name + "-html").Option("missingkey=error").Parse(htmlBody)
		if err != nil {
			return nil, fmt.Errorf("mail HTML template: %w", err)
		}
	}
	if templates.text == nil && templates.html == nil {
		return nil, fmt.Errorf("mail template requires text or HTML content")
	}
	return templates, nil
}

func (templates *Templates) Render(data any) (subject, textBody, htmlBody string, err error) {
	if templates == nil || templates.subject == nil {
		return "", "", "", fmt.Errorf("mail templates are required")
	}
	execute := func(template interface{ Execute(io.Writer, any) error }) (string, error) {
		var output bytes.Buffer
		if err := template.Execute(&output, data); err != nil {
			return "", err
		}
		return output.String(), nil
	}
	if subject, err = execute(templates.subject); err != nil {
		return "", "", "", fmt.Errorf("render mail subject: %w", err)
	}
	if templates.text != nil {
		if textBody, err = execute(templates.text); err != nil {
			return "", "", "", fmt.Errorf("render mail text: %w", err)
		}
	}
	if templates.html != nil {
		if htmlBody, err = execute(templates.html); err != nil {
			return "", "", "", fmt.Errorf("render mail HTML: %w", err)
		}
	}
	return subject, textBody, htmlBody, nil
}
