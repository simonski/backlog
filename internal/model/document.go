package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type DocumentUpdate struct {
	Root     string `json:"root"`
	Name     string `json:"name"`
	Markdown string `json:"markdown"`
}

func (document DocumentUpdate) Validate() error {
	if err := ValidateRoot(document.Root); err != nil {
		return err
	}
	name := document.Name
	if name == "" || name == "." || name == ".." || strings.TrimSpace(name) != name || len(name) > 200 ||
		!utf8.ValidString(name) || strings.ContainsAny(name, `/\`) || strings.ContainsFunc(name, unicode.IsControl) {
		return Invalid("document name must be 1 to 200 bytes with no path separators, control characters, or surrounding spaces")
	}
	if !utf8.ValidString(document.Markdown) {
		return Invalid("document must be UTF-8 Markdown")
	}
	return nil
}

type DocumentResult struct {
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

type DocumentClear struct {
	Root string `json:"root"`
}

type DocumentClearResult struct {
	ProjectID int64 `json:"project_id"`
	Removed   int64 `json:"removed"`
}
