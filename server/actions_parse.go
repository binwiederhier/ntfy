package server

import (
	"errors"
	"fmt"
	"heckel.io/ntfy/util"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Heavily inspired by https://go.dev/src/text/template/parse/lex.go
// And thanks to Rob Pike (for Go, but also) for https://www.youtube.com/watch?v=HxaD_trXwRE

// action=view, label="Look ma, commas and \"quotes\" too", url=https://..

// "Look ma, a button",
// Look ma a button
// label=Look ma a=button
// label="Look ma, a button"
// "Look ma, \"quotes\""
// label="Look ma, \"quotes\""
// label=,

func parseActionsFromSimpleNew(s string) ([]*action, error) {
	if !utf8.ValidString(s) {
		return nil, errors.New("invalid string")
	}
	parser := &actionParser{
		pos:   0,
		input: s,
	}
	return parser.Parse()
}

type actionParser struct {
	input string
	pos   int
}

const eof = rune(0)

func (p *actionParser) Parse() ([]*action, error) {
	println("------------------------")
	actions := make([]*action, 0)
	for !p.eof() {
		a, err := p.parseAction()
		if err != nil {
			return nil, err
		} else if a == nil {
			return actions, err
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func (p *actionParser) parseAction() (*action, error) {
	println("parseAction")
	newAction := &action{
		Headers: make(map[string]string),
		Extras:  make(map[string]string),
	}
	section := 0
	for {
		key, value, last, err := p.parseSection()
		fmt.Printf("--> key=%s, value=%s, last=%t, err=%#v\n", key, value, last, err)
		if err != nil {
			return nil, err
		} else if key == "" && section == 0 {
			key = "action"
		} else if key == "" && section == 1 {
			key = "label"
		} else if key == "" && section == 2 && util.InStringList([]string{"view", "http"}, newAction.Action) {
			key = "url"
		} else if key == "" {
			return nil, wrapErrHTTP(errHTTPBadRequestActionsInvalid, "term '%s' unknown", value)
		}
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
					return nil, wrapErrHTTP(errHTTPBadRequestActionsInvalid, "'clear=%s' not allowed", value)
				}
				newAction.Clear = lvalue == "true" || lvalue == "yes" || lvalue == "1"
			case "url":
				newAction.URL = value
			case "method":
				newAction.Method = value
			case "body":
				newAction.Body = value
			default:
				return nil, wrapErrHTTP(errHTTPBadRequestActionsInvalid, "key '%s' unknown", key)
			}
		}
		p.slurpSpaces()
		if last {
			return newAction, nil
		}
		section++
	}
}

func (p *actionParser) parseSection() (key string, value string, last bool, err error) {
	fmt.Printf("parseSection, pos=%d, len(input)=%d, input[pos:]=%s\n", p.pos, len(p.input), p.input[p.pos:])
	p.slurpSpaces()
	key = p.parseKey()
	r, w := p.peek()
	if r == eof || r == ';' || r == ',' {
		p.pos += w
		last = r == ';' || r == eof
		return
	} else if r == '"' {
		value, last, err = p.parseQuotedValue()
		return
	}
	value, last = p.parseValue()
	return
}

func (p *actionParser) parseValue() (value string, last bool) {
	start := p.pos
	for {
		r, w := p.peek()
		if r == eof || r == ';' || r == ',' {
			last = r == ';' || r == eof
			value = p.input[start:p.pos]
			p.pos += w
			return
		}
		p.pos += w
	}
}

func (p *actionParser) parseQuotedValue() (value string, last bool, err error) {
	p.pos++
	start := p.pos
	var prev rune
	for {
		r, w := p.peek()
		if r == eof {
			err = errors.New("unexpected end of input")
			return
		} else if r == '"' && prev != '\\' {
			value = p.input[start:p.pos]
			p.pos += w

			// Advance until after "," or ";"
			p.slurpSpaces()
			r, w := p.peek()
			last = r == ';' || r == eof
			if r != eof && r != ';' && r != ',' {
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

var keyRegex = regexp.MustCompile(`^[-.\w]+=`)

func (p *actionParser) parseKey() string {
	key := keyRegex.FindString(p.input[p.pos:])
	if key != "" {
		p.pos += len(key)
		return key[:len(key)-1]
	}
	return key
}

func (p *actionParser) peek() (rune, int) {
	if p.pos >= len(p.input) {
		return eof, 0
	}
	return utf8.DecodeRuneInString(p.input[p.pos:])
}

func (p *actionParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *actionParser) slurpSpaces() {
	for {
		r, w := p.peek()
		if r == eof || !isSpace(r) {
			return
		}
		p.pos += w
	}
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}
