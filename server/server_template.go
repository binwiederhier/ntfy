package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template/parse"
	"time"

	"gopkg.in/yaml.v2"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/template/gotext"
	"heckel.io/ntfy/v2/util"
	"heckel.io/ntfy/v2/util/sprig"
)

var (
	//go:embed templates
	templatesFs  embed.FS // Contains template config files (e.g. grafana.yml, github.yml, ...)
	templatesDir = "templates"

	templateNameRegex = regexp.MustCompile(`^[-_A-Za-z0-9]+$`)

	// templatePrintfLargeSizeRegex matches a printf directive whose width or precision is a star
	// (taken from an argument) or has four or more digits, i.e. is at least 1000. It deliberately
	// scans the flag/width/precision characters after a % without requiring a well-formed
	// directive: fmt pads even malformed ones (e.g. "%000 9999999#" emits 10 MB), so anything
	// unrecognized must still be caught.
	templatePrintfLargeSizeRegex = regexp.MustCompile(`%[-+# 0-9.*\[\]]*(\*|[0-9]{4})`)

	// templateMaxExecutionTime is the wall-clock deadline for a single template render, a DoS guard
	// (GHSA-rhwf-xgc9-m9fp). It is a var (not a const) solely so tests can raise it; it is never
	// mutated in production.
	templateMaxExecutionTime = 100 * time.Millisecond
)

const (
	templateMaxOutputBytes   = 1024 * 1024 // Maximum number of bytes a template can output, used to prevent DoS attacks
	templateMaxTemplateBytes = 32 * 1024   // Maximum size of a template (inline or from a template file), used to prevent DoS attacks
	templateFileExtension    = ".yml"      // Template files must end with this extension
)

func (s *Server) handleBodyAsTemplatedTextMessage(ctx context.Context, m *model.Message, template templateMode, body *util.PeekedReadCloser, priorityStr string) error {
	body, err := util.Peek(body, max(s.config.MessageSizeLimit, jsonBodyBytesLimit))
	if err != nil {
		return err
	} else if body.LimitReached {
		return errHTTPEntityTooLargeJSONBody
	}
	peekedBody := strings.TrimSpace(string(body.PeekedBytes))
	if template.FileMode() {
		if err := s.renderTemplateFromFile(ctx, m, template.FileName(), peekedBody); err != nil {
			return err
		}
	} else {
		if err := s.renderTemplateFromParams(ctx, m, peekedBody, priorityStr); err != nil {
			return err
		}
	}
	if len(m.Title) > s.config.MessageSizeLimit || len(m.Message) > s.config.MessageSizeLimit {
		return errHTTPBadRequestTemplateMessageTooLarge
	}
	return nil
}

// renderTemplateFromFile transforms the JSON message body according to a template from the filesystem.
// The template file must be in the templates directory, or in the configured template directory.
func (s *Server) renderTemplateFromFile(ctx context.Context, m *model.Message, templateName, peekedBody string) error {
	if !templateNameRegex.MatchString(templateName) {
		return errHTTPBadRequestTemplateFileNotFound
	}
	templateContent, _ := templatesFs.ReadFile(filepath.Join(templatesDir, templateName+templateFileExtension)) // Read from the embedded filesystem first
	if s.config.TemplateDir != "" {
		if b, _ := os.ReadFile(filepath.Join(s.config.TemplateDir, templateName+templateFileExtension)); len(b) > 0 {
			templateContent = b
		}
	}
	if len(templateContent) == 0 {
		return errHTTPBadRequestTemplateFileNotFound
	}
	var tpl templateFile
	if err := yaml.Unmarshal(templateContent, &tpl); err != nil {
		return errHTTPBadRequestTemplateFileInvalid
	}
	var err error
	if tpl.Message != nil {
		if m.Message, err = s.renderTemplate(ctx, templateName+" (message)", *tpl.Message, peekedBody); err != nil {
			return err
		}
	}
	if tpl.Title != nil {
		if m.Title, err = s.renderTemplate(ctx, templateName+" (title)", *tpl.Title, peekedBody); err != nil {
			return err
		}
	}
	if tpl.Priority != nil {
		renderedPriority, err := s.renderTemplate(ctx, templateName+" (priority)", *tpl.Priority, peekedBody)
		if err != nil {
			return err
		}
		if m.Priority, err = util.ParsePriority(renderedPriority); err != nil {
			return errHTTPBadRequestPriorityInvalid
		}
	}
	return nil
}

// renderTemplateFromParams transforms the JSON message body according to the inline template in the
// message, title, and priority parameters.
func (s *Server) renderTemplateFromParams(ctx context.Context, m *model.Message, peekedBody string, priorityStr string) error {
	var err error
	if m.Message, err = s.renderTemplate(ctx, "priority query parameter", m.Message, peekedBody); err != nil {
		return err
	}
	if m.Title, err = s.renderTemplate(ctx, "title query parameter", m.Title, peekedBody); err != nil {
		return err
	}
	if priorityStr != "" {
		renderedPriority, err := s.renderTemplate(ctx, "priority query parameter", priorityStr, peekedBody)
		if err != nil {
			return err
		}
		if m.Priority, err = util.ParsePriority(renderedPriority); err != nil {
			return errHTTPBadRequestPriorityInvalid
		}
	}
	return nil
}

// renderTemplate renders a template with the given JSON source data.
func (s *Server) renderTemplate(ctx context.Context, name, tpl, source string) (string, error) {
	if len(tpl) > templateMaxTemplateBytes {
		return "", errHTTPBadRequestTemplateTooLarge
	}
	var data any
	if err := json.Unmarshal([]byte(source), &data); err != nil {
		return "", errHTTPBadRequestTemplateMessageNotJSON
	}
	t, err := gotext.New("").Funcs(sprig.TxtFuncMap()).Funcs(gotext.FuncMap{"printf": templatePrintf}).Parse(tpl)
	if err != nil {
		return "", errHTTPBadRequestTemplateInvalid.Wrap("%s", err.Error())
	}
	if templateUsesDisallowedFeatures(t) {
		return "", errHTTPBadRequestTemplateDisallowedFunctionCalls
	}
	// Bail out of runaway templates (GHSA-rhwf-xgc9-m9fp). The deadline starts here, after the body
	// has already been read, so a slow upload is not counted against it. Deriving from the request
	// context means a client disconnect aborts the render too.
	execCtx, cancel := context.WithTimeout(ctx, templateMaxExecutionTime)
	defer cancel()
	var buf bytes.Buffer
	limitWriter := util.NewLimitWriter(&buf, util.NewFixedLimiter(templateMaxOutputBytes))
	if err := t.ExecuteContext(execCtx, limitWriter, data); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", errHTTPBadRequestTemplateExecutionTimeout
		}
		return "", errHTTPBadRequestTemplateExecuteFailed.Wrap("template %s: %s", name, err.Error())
	}
	return strings.TrimSpace(strings.ReplaceAll(buf.String(), "\\n", "\n")), nil // replace any remaining "\n" (those outside of template curly braces) with newlines
}

// templateUsesDisallowedFeatures reports whether the parsed template defines or invokes a
// sub-template ({{define}}/{{block}}/{{template}}) or uses the {{call}} builtin. None are useful for
// ntfy's JSON-data templates. Checking the parse tree (rather than the raw string) catches every
// syntactic form -- e.g. {{if call .x}} or {{$y := call .x}} -- that a regex would miss.
func templateUsesDisallowedFeatures(t *gotext.Template) bool {
	if len(t.Templates()) > 1 { // {{define}}/{{block}} create additional associated templates
		return true
	}
	return treeContainsDisallowedNode(t.Root)
}

// treeContainsDisallowedNode reports whether the parse tree contains a {{template}}/{{block}}
// invocation or a {{call}} builtin, descending into pipes and command arguments (where {{call}} can
// appear anywhere a function is allowed).
func treeContainsDisallowedNode(node parse.Node) bool {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return false
		}
		for _, child := range n.Nodes {
			if treeContainsDisallowedNode(child) {
				return true
			}
		}
	case *parse.ActionNode:
		return treeContainsDisallowedNode(n.Pipe)
	case *parse.RangeNode:
		return treeContainsDisallowedNode(n.Pipe) || treeContainsDisallowedNode(n.List) || treeContainsDisallowedNode(n.ElseList)
	case *parse.IfNode:
		return treeContainsDisallowedNode(n.Pipe) || treeContainsDisallowedNode(n.List) || treeContainsDisallowedNode(n.ElseList)
	case *parse.WithNode:
		return treeContainsDisallowedNode(n.Pipe) || treeContainsDisallowedNode(n.List) || treeContainsDisallowedNode(n.ElseList)
	case *parse.TemplateNode: // {{template}} or {{block}} invocation
		return true
	case *parse.ChainNode: // A term followed by field accesses, e.g. (call .x).y
		return treeContainsDisallowedNode(n.Node)
	case *parse.PipeNode:
		if n == nil {
			return false
		}
		for _, cmd := range n.Cmds {
			if treeContainsDisallowedNode(cmd) {
				return true
			}
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			if treeContainsDisallowedNode(arg) {
				return true
			}
		}
	case *parse.IdentifierNode: // a function name; {{call}} is the disallowed builtin
		return n.Ident == "call"
	}
	return false
}

// templatePrintf is the template builtin printf, guarded against memory amplification: fmt
// allows widths and precisions up to 1e6 per verb, so a small template like
// {{printf "%999999d%999999d..." ...}} can allocate gigabytes inside a single fmt call -- and the
// executor's cancellation context is only checked between template nodes, never inside one.
// Widths and precisions of 1000 or more are therefore rejected, as is the star (*) form, which
// takes the width from an argument. Combined with the template size limit, this bounds a single
// render to a few MB. Registered via Funcs, which takes precedence over the builtin, and checked
// at call time so a format string assembled during execution is covered too.
func templatePrintf(format string, args ...any) (string, error) {
	if templatePrintfLargeSizeRegex.MatchString(strings.ReplaceAll(format, "%%", "")) { // Strip escaped percent signs, they take no width
		return "", errors.New("printf width or precision too large")
	}
	return fmt.Sprintf(format, args...), nil
}
