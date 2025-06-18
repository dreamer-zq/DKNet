package templates

import (
	"embed"
	"io/fs"
)

//go:embed *.tmpl
var templateFS embed.FS

// GetTemplateFS returns the embedded template filesystem
func GetTemplateFS() fs.FS {
	return templateFS
}

// GetTemplate reads a template file and returns its content
func GetTemplate(name string) ([]byte, error) {
	return templateFS.ReadFile(name)
}
