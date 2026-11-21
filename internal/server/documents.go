package server

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"harness.engineering/internal/model"
)

var documentPage = template.Must(template.ParseFS(files, "web/document.html"))

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

func documentPath(projectID int64, name string) string {
	return fmt.Sprintf("/projects/%d/docs/%s", projectID, url.PathEscape(name))
}

func (app server) saveDocument(w http.ResponseWriter, r *http.Request) {
	var document model.DocumentUpdate
	if !readJSONRequest(w, r, &document) {
		return
	}
	projectID, err := app.store.SaveDocument(r.Context(), document)
	result := model.DocumentResult{ProjectID: projectID, Name: document.Name, Path: documentPath(projectID, document.Name)}
	writeAPIResponse(w, result, err, "save document")
}

func (app server) clearDocuments(w http.ResponseWriter, r *http.Request) {
	var input model.DocumentClear
	if !readJSONRequest(w, r, &input) {
		return
	}
	result, err := app.store.ClearDocuments(r.Context(), input.Root)
	writeAPIResponse(w, result, err, "clear documents")
}

func (app server) document(w http.ResponseWriter, r *http.Request) {
	id, err := projectID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	doc, err := app.store.Document(r.Context(), id, r.PathValue("name"))
	if err != nil {
		pageError(w, r, err)
		return
	}
	var rendered bytes.Buffer
	if err := markdownRenderer.Convert([]byte(doc.Markdown), &rendered); err != nil {
		pageError(w, r, err)
		return
	}
	view := struct {
		ProjectID   int64
		ProjectName string
		Name        string
		Content     template.HTML
	}{id, filepath.Base(doc.Root), doc.Name, template.HTML(rendered.String())} // Goldmark's default renderer omits raw HTML and unsafe links.
	var output bytes.Buffer
	if err := documentPage.Execute(&output, view); err != nil {
		pageError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := output.WriteTo(w); err != nil {
		log.Printf("write document: %v", err)
	}
}
