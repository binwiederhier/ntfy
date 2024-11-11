package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"heckel.io/ntfy/v2/util"
)

type templateFeature int

const templateExtension = ".tpl"

var (
	templateEnabledDefault = "disabled"
	templateEnabledInline  = "inline"
	templateEnabledServer  = "server"
)

const (
	templateFeatureDisabled templateFeature = iota
	templateFeatureInline
	templateFeatureServer
)

var templateFeatureNames = map[templateFeature]string{
	templateFeatureDisabled: templateEnabledDefault,
	templateFeatureInline:   templateEnabledInline,
	templateFeatureServer:   templateEnabledServer,
}

func parseTemplateFeature(v string) templateFeature {
	if isBoolValue(v) {
		// backwards-compatibility support
		if toBool(v) {
			return templateFeatureInline
		}
	} else if v == templateEnabledInline {
		return templateFeatureInline
	} else if v == templateEnabledServer {
		return templateFeatureServer
	}

	return templateFeatureDisabled
}

// String returns a human readable description of the template feature setting.
// invalid values are folded into the [templateEnabledDefault] state.
func (f templateFeature) String() string {
	if n, ok := templateFeatureNames[f]; ok {
		return n
	}

	return templateEnabledDefault
}

// listTemplates returns the base names without extensions
// of all files in the configured templates directory.
func (s *Server) listTemplates() ([]string, error) {
	templates := []string{}
	fileSystem := os.DirFS(s.config.TemplateDirectory)
	walker := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if !d.Type().IsRegular() {
			return nil
		}

		name := strings.TrimSuffix(d.Name(), templateExtension)
		if name == d.Name() {
			// entry does not have the suffix
			return nil
		}

		templates = append(templates, name)

		return nil
	}

	if err := fs.WalkDir(fileSystem, ".", walker); err != nil {
		return nil, err
	}

	return templates, nil
}

// serveTemplate writes the file content of the given template into the writer.
// the filename is generated using [Server.templateFilePath].
// errors encountered during preparations are returned, errors encountered during serving
// the file content are written to the response writer instead.
func (s *Server) serveTemplate(w http.ResponseWriter, r *http.Request, name string) error {
	f, err := os.Open(s.templateFilePath(name))
	if err != nil {
		return err
	}
	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		return err
	}

	// prevent serve content from wrongly identifying the content type
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, d.Name(), d.ModTime(), f)
	return nil
}

// templateFilePath creates a filepath pointing to a file
// in the servers template directory. to prevent directory
// traversal, the input value is cleaned/sanitized. name must
// not have a file extension.
func (s *Server) templateFilePath(name string) string {
	// prevent directory traversal
	filename := filepath.Base(filepath.Clean(name)) + templateExtension
	return filepath.Join(s.config.TemplateDirectory, filename)
}

// replaceTemplate loads the given value as template and renders
// it using the provided context/data. the method returns two errors; the first one
// is ment for logging and internal insight as it might contain potential sensitive
// information. the second error is intended to be served to clients.
func (s *Server) replaceTemplate(tpl string, data any) (string, error, error) {
	if templateDisallowedRegex.MatchString(tpl) {
		return "", nil, errHTTPBadRequestTemplateDisallowedFunctionCalls
	}

	t, err := template.New("").Parse(tpl)
	if err != nil {
		return "", err, errHTTPBadRequestTemplateInvalid
	}
	var buf bytes.Buffer
	if err := t.Execute(util.NewTimeoutWriter(&buf, templateMaxExecutionTime), data); err != nil {
		return "", err, errHTTPBadRequestTemplateExecuteFailed
	}
	return buf.String(), nil, nil
}

// replaceTemplateFile loads the template identified by the given name and renders
// it using the provided context/data. the method returns two errors; the first one
// is ment for logging and internal insight as it might contain potential sensitive
// information. the second error is intended to be served to clients.
func (s *Server) replaceTemplateFile(name string, data any) (string, error, error) {
	if name == "" {
		// instead of bailing with an error, we simply tread the request as a no-op
		// and return no data
		return "", nil, nil
	}

	t, err := template.ParseFiles(s.templateFilePath(name))
	if err != nil {
		return "", err, errHTTPBadRequestTemplateInvalid
	}
	var buf bytes.Buffer
	if err := t.Execute(util.NewTimeoutWriter(&buf, templateMaxExecutionTime), data); err != nil {
		return "", err, errHTTPBadRequestTemplateExecuteFailed
	}
	return buf.String(), nil, nil
}
