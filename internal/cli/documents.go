package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
	"harness.engineering/internal/wire"
)

func uploadDocument(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) error {
	flags := newFlags("doc", diagnostics, " (-file PATH [-name NAME] | -clear) [options]")
	path := flags.String("file", "", "Markdown file or - for standard input")
	name := flags.String("name", "", "document name (defaults to the file name)")
	clear := flags.Bool("clear", false, "remove every uploaded document for this project")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "local server host:port")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if *clear {
		if *path != "" || *name != "" {
			return errors.New("-clear cannot be combined with -file or -name")
		}
		return clearProjectDocuments(ctx, address, *asJSON, out, diagnostics)
	}
	switch *path {
	case "":
		return errors.New("-file is required")
	case "-":
		if *name == "" || input == nil {
			return errors.New("-file - requires -name and Markdown on standard input")
		}
	default:
		file, err := os.Open(*path)
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
		if *name == "" {
			*name = filepath.Base(*path)
		}
	}
	source, err := io.ReadAll(io.LimitReader(input, wire.MaxReportBytes+1))
	if err != nil {
		return err
	}
	if len(source) > wire.MaxReportBytes {
		return errors.New("document exceeds 1 MiB")
	}
	root, err := projectRoot(ctx)
	if err != nil {
		return err
	}
	document := model.DocumentUpdate{Root: root, Name: *name, Markdown: string(source)}
	if err := document.Validate(); err != nil {
		return err
	}
	var receipt model.DocumentResult
	if err := requestServer(ctx, address, http.MethodPost, "/api/documents", document, &receipt, diagnostics); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, receipt)
	}
	_, err = fmt.Fprintf(out, "Document: %s\nPath:     %s\n", receipt.Name, receipt.Path)
	return err
}

func clearProjectDocuments(ctx context.Context, address string, asJSON bool, out, diagnostics io.Writer) error {
	root, err := projectRoot(ctx)
	if err != nil {
		return err
	}
	var result model.DocumentClearResult
	if err := requestServer(ctx, address, http.MethodDelete, "/api/documents", model.DocumentClear{Root: root}, &result, diagnostics); err != nil {
		return err
	}
	if asJSON {
		return writeJSON(out, result)
	}
	_, err = fmt.Fprintf(out, "Project ID:        %d\nDocuments removed: %d\n", result.ProjectID, result.Removed)
	return err
}
