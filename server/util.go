package server

import (
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
)

const (
	actionIDLength = 10
	actionsMax     = 3
)

func readBoolParam(r *http.Request, defaultValue bool, names ...string) bool {
	value := strings.ToLower(readParam(r, names...))
	if value == "" {
		return defaultValue
	}
	return value == "1" || value == "yes" || value == "true"
}

func readParam(r *http.Request, names ...string) string {
	value := readHeaderParam(r, names...)
	if value != "" {
		return value
	}
	return readQueryParam(r, names...)
}

func readHeaderParam(r *http.Request, names ...string) string {
	for _, name := range names {
		value := r.Header.Get(name)
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readQueryParam(r *http.Request, names ...string) string {
	for _, name := range names {
		value := r.URL.Query().Get(strings.ToLower(name))
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

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

	// Add ID field
	for i := range actions {
		actions[i].ID = util.RandomString(actionIDLength)
	}

	// Validate
	if len(actions) > actionsMax {
		return nil, fmt.Errorf("too many actions, only %d allowed", actionsMax)
	}
	for _, action := range actions {
		if !util.InStringList([]string{"view", "broadcast", "http"}, action.Action) {
			return nil, fmt.Errorf("cannot parse actions: action '%s' unknown", action.Action)
		} else if action.Label == "" {
			return nil, fmt.Errorf("cannot parse actions: label must be set")
		} else if util.InStringList([]string{"view", "http"}, action.Action) && action.URL == "" {
			return nil, fmt.Errorf("parameter 'url' is required for action '%s'", action.Action)
		}
	}

	return actions, nil
}

func parseActionsFromJSON(s string) ([]*action, error) {
	actions := make([]*action, 0)
	if err := json.Unmarshal([]byte(s), &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

func parseActionsFromSimple(s string) ([]*action, error) {
	actions := make([]*action, 0)
	rawActions := util.SplitNoEmpty(s, ";")
	for _, rawAction := range rawActions {
		newAction := &action{
			Headers: make(map[string]string),
			Extras:  make(map[string]string),
		}
		parts := util.SplitNoEmpty(rawAction, ",")
		if len(parts) < 3 {
			return nil, fmt.Errorf("cannot parse action: action requires at least keys 'action', 'label' and one parameter: %s", rawAction)
		}
		for i, part := range parts {
			key, value := util.SplitKV(part, "=")
			if key == "" && i == 0 {
				newAction.Action = value
			} else if key == "" && i == 1 {
				newAction.Label = value
			} else if key == "" && util.InStringList([]string{"view", "http"}, newAction.Action) && i == 2 {
				newAction.URL = value
			} else if strings.HasPrefix(key, "headers.") {
				newAction.Headers[strings.TrimPrefix(key, "headers.")] = value
			} else if strings.HasPrefix(key, "extras.") {
				newAction.Extras[strings.TrimPrefix(key, "extras.")] = value
			} else if key != "" {
				switch strings.ToLower(key) {
				case "action":
					newAction.Action = value
				case "label":
					newAction.Label = value
				case "clear":
					lvalue := strings.ToLower(value)
					newAction.Clear = lvalue == "true" || lvalue == "yes" || lvalue == "1"
				case "url":
					newAction.URL = value
				case "method":
					newAction.Method = value
				case "body":
					newAction.Body = value
				default:
					return nil, fmt.Errorf("cannot parse action: key '%s' not supported, please use JSON format instead", part)
				}
			} else {
				return nil, fmt.Errorf("cannot parse action: unknown phrase '%s'", part)
			}
		}
		actions = append(actions, newAction)
	}
	return actions, nil
}
