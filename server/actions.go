package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"heckel.io/ntfy/util"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	actionIDLength = 10
	actionEOF      = rune(0)
	actionsMax     = 3
)

const (
	actionView      = "view"
	actionBroadcast = "broadcast"
	actionHTTP      = "http"
)

var (
	actionsAll      = []string{actionView, actionBroadcast, actionHTTP}
	actionsWithURL  = []string{actionView, actionHTTP}
	actionsKeyRegex = regexp.MustCompile(`^([-.\w]+)\s*=\s*`)
)

type actionParser struct {
	input string
	pos   int
}

// parseActions parses the actions string as described in https://ntfy.sh/docs/publish/#action-buttons.
// It supports both a JSON representation (if the string begins with "[", see parseActionsFromJSON),
// and the "simple" format, which is more human-readable, but harder to parse (see parseActionsFromSimple).
func parseActions(s string) (actions []*action, err error) {
	// Parse JSON or simple format
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		actions, err = parseActionsFromJSON(s)
	} else {
		actions, err = parseActionsFromSimple(s)
	}
	if err != nil {
		return nil, err
	}

	// Add ID field, ensure correct uppercase/lowercase
	for i := range actions {
		actions[i].ID = util.RandomString(actionIDLength)
		actions[i].Action = strings.ToLower(actions[i].Action)
		actions[i].Method = strings.ToUpper(actions[i].Method)
	}

	// Validate
	if len(actions) > actionsMax {
		return nil, fmt.Errorf("only %d actions allowed", actionsMax)
	}
	for _, action := range actions {
		if !util.InStringList(actionsAll, action.Action) {
			return nil, fmt.Errorf("parameter 'action' cannot be '%s', valid values are 'view', 'broadcast' and 'http'", action.Action)
		} else if action.Label == "" {
			return nil, fmt.Errorf("parameter 'label' is required")
		} else if util.InStringList(actionsWithURL, action.Action) && action.URL == "" {
			return nil, fmt.Errorf("parameter 'url' is required for action '%s'", action.Action)
		} else if action.Action == actionHTTP && util.InStringList([]string{"GET", "HEAD"}, action.Method) && action.Body != "" {
			return nil, fmt.Errorf("parameter 'body' cannot be set if method is %s", action.Method)
		}
	}

	return actions, nil
}

// parseActionsFromJSON converts a JSON array into an array of actions
func parseActionsFromJSON(s string) ([]*action, error) {
	actions := make([]*action, 0)
	if err := json.Unmarshal([]byte(s), &actions); err != nil {
		return nil, fmt.Errorf("JSON error: %w", err)
	}
	return actions, nil
}

// parseActionsFromSimple parses the "simple" actions string (as described in
// https://ntfy.sh/docs/publish/#action-buttons), into an array of actions.
//
// It can parse an actions string like this:
//    view, "Look ma, commas and \"quotes\" too", url=https://..; action=broadcast, ...
//
// It works by advancing the position ("pos") through the input string ("input").
//
// The parser is heavily inspired by https://go.dev/src/text/template/parse/lex.go (which
// is described by Rob Pike in this video: https://www.youtube.com/watch?v=HxaD_trXwRE),
// though it does not use state functions at all.
//
// Other resources:
//   https://adampresley.github.io/2015/04/12/writing-a-lexer-and-parser-in-go-part-1.html
//   https://github.com/adampresley/sample-ini-parser/blob/master/services/lexer/lexer/Lexer.go
//   https://github.com/benbjohnson/sql-parser/blob/master/scanner.go
//   https://blog.gopheracademy.com/advent-2014/parsers-lexers/
func parseActionsFromSimple(s string) ([]*action, error) {
	if !utf8.ValidString(s) {
		return nil, errors.New("invalid utf-8 string")
	}
	parser := &actionParser{
		pos:   0,
		input: s,
	}
	return parser.Parse()
}

// Parse loops trough parseAction() until the end of the string is reached
func (p *actionParser) Parse() ([]*action, error) {
	actions := make([]*action, 0)
	for !p.eof() {
		a, err := p.parseAction()
		if err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, nil
}

// parseAction parses the individual sections of an action using parseSection into key/value pairs,
// and then uses populateAction to interpret the keys/values. The function terminates
// when EOF or ";" is reached.
func (p *actionParser) parseAction() (*action, error) {
	a := newAction()
	section := 0
	for {
		key, value, last, err := p.parseSection()
		if err != nil {
			return nil, err
		}
		if err := populateAction(a, section, key, value); err != nil {
			return nil, err
		}
		p.slurpSpaces()
		if last {
			return a, nil
		}
		section++
	}
}

// populateAction is the "business logic" of the parser. It applies the key/value
// pair to the action instance.
func populateAction(newAction *action, section int, key, value string) error {
	// Auto-expand keys based on their index
	if key == "" && section == 0 {
		key = "action"
	} else if key == "" && section == 1 {
		key = "label"
	} else if key == "" && section == 2 && util.InStringList(actionsWithURL, newAction.Action) {
		key = "url"
	}

	// Validate
	if key == "" {
		return fmt.Errorf("term '%s' unknown", value)
	}

	// Populate
	if strings.HasPrefix(key, "headers.") {
		newAction.Headers[strings.TrimPrefix(key, "headers.")] = value
	} else if strings.HasPrefix(key, "extras.") {
		newAction.Extras[strings.TrimPrefix(key, "extras.")] = value
	} else {
		switch strings.ToLower(key) {
		case "action":
			newAction.Action = value
		case "label":
			newAction.Label = value
		case "clear":
			lvalue := strings.ToLower(value)
			if !util.InStringList([]string{"true", "yes", "1", "false", "no", "0"}, lvalue) {
				return fmt.Errorf("parameter 'clear' cannot be '%s', only boolean values are allowed (true/yes/1/false/no/0)", value)
			}
			newAction.Clear = lvalue == "true" || lvalue == "yes" || lvalue == "1"
		case "url":
			newAction.URL = value
		case "method":
			newAction.Method = value
		case "body":
			newAction.Body = value
		case "intent":
			newAction.Intent = value
		default:
			return fmt.Errorf("key '%s' unknown", key)
		}
	}
	return nil
}

// parseSection parses a section ("key=value") and returns a key/value pair. It terminates
// when EOF or "," is reached.
func (p *actionParser) parseSection() (key string, value string, last bool, err error) {
	p.slurpSpaces()
	key = p.parseKey()
	r, w := p.peek()
	if isSectionEnd(r) {
		p.pos += w
		last = isLastSection(r)
		return
	} else if r == '"' || r == '\'' {
		value, last, err = p.parseQuotedValue(r)
		return
	}
	value, last = p.parseValue()
	return
}

// parseKey uses a regex to determine whether the current position is a key definition ("key =")
// and returns the key if it is, or an empty string otherwise.
func (p *actionParser) parseKey() string {
	matches := actionsKeyRegex.FindStringSubmatch(p.input[p.pos:])
	if len(matches) == 2 {
		p.pos += len(matches[0])
		return matches[1]
	}
	return ""
}

// parseValue reads the input until EOF, "," or ";" and returns the value string. Unlike parseQuotedValue,
// this function does not support "," or ";" in the value itself, and spaces in the beginning and end of the
// string are trimmed.
func (p *actionParser) parseValue() (value string, last bool) {
	start := p.pos
	for {
		r, w := p.peek()
		if isSectionEnd(r) {
			last = isLastSection(r)
			value = strings.TrimSpace(p.input[start:p.pos])
			p.pos += w
			return
		}
		p.pos += w
	}
}

// parseQuotedValue reads the input until it finds an unescaped end quote character ("), and then
// advances the position beyond the section end. It supports quoting strings using backslash (\).
func (p *actionParser) parseQuotedValue(quote rune) (value string, last bool, err error) {
	p.pos++
	start := p.pos
	var prev rune
	for {
		r, w := p.peek()
		if r == actionEOF {
			err = fmt.Errorf("unexpected end of input, quote started at position %d", start)
			return
		} else if r == quote && prev != '\\' {
			value = strings.ReplaceAll(p.input[start:p.pos], "\\"+string(quote), string(quote)) // \" -> "
			p.pos += w

			// Advance until section end (after "," or ";")
			p.slurpSpaces()
			r, w := p.peek()
			last = isLastSection(r)
			if !isSectionEnd(r) {
				err = fmt.Errorf("unexpected character '%c' at position %d", r, p.pos)
				return
			}
			p.pos += w
			return
		}
		prev = r
		p.pos += w
	}
}

// slurpSpaces reads all space characters and advances the position
func (p *actionParser) slurpSpaces() {
	for {
		r, w := p.peek()
		if r == actionEOF || !isSpace(r) {
			return
		}
		p.pos += w
	}
}

// peek returns the next run and its width
func (p *actionParser) peek() (rune, int) {
	if p.eof() {
		return actionEOF, 0
	}
	return utf8.DecodeRuneInString(p.input[p.pos:])
}

// eof returns true if the end of the input has been reached
func (p *actionParser) eof() bool {
	return p.pos >= len(p.input)
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isSectionEnd(r rune) bool {
	return r == actionEOF || r == ';' || r == ','
}

func isLastSection(r rune) bool {
	return r == actionEOF || r == ';'
}
