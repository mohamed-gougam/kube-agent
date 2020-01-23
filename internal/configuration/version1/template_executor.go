package version1

import (
	"bytes"
	"path"
	"text/template"
)

// TemplateExecutor executes NGINX configuration templates.
type TemplateExecutor struct {
	tcpServerTemplate *template.Template
}

// NewTemplateExecutor creates a TemplateExecutor.
func NewTemplateExecutor(tcpsTemplatePath string) (*TemplateExecutor, error) {
	// template name must be the base name of the template file https://golang.org/pkg/text/template/#Template.ParseFiles
	tcpServerTemplate, err := template.New(path.Base(tcpsTemplatePath)).ParseFiles(tcpsTemplatePath)
	if err != nil {
		return nil, err
	}

	return &TemplateExecutor{
		tcpServerTemplate: tcpServerTemplate,
	}, nil
}

// ExecuteTCPServerConfigTemplate generates the content of the main NGINX configuration file.
func (te *TemplateExecutor) ExecuteTCPServerConfigTemplate(cfg *TCPServerConf) ([]byte, error) {
	var configBuffer bytes.Buffer
	err := te.tcpServerTemplate.Execute(&configBuffer, cfg)

	return configBuffer.Bytes(), err
}
