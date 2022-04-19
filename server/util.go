package server

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
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
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		actions, err = parseActionsFromJSON(s)
	} else {
		actions, err = parseActionsFromSimple(s)
	}
	if err != nil {
		return nil, err
	}
	for i := range actions {
		actions[i].ID = util.RandomString(actionIDLength)
		if !util.InStringList([]string{"view", "broadcast", "http"}, actions[i].Action) {
			return nil, fmt.Errorf("cannot parse actions: action '%s' unknown", actions[i].Action)
		} else if actions[i].Label == "" {
			return nil, fmt.Errorf("cannot parse actions: label must be set")
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
		newAction := &action{}
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
			} else if key == "" && i == 2 {
				newAction.URL = value // This works, because both "http" and "view" need a URL
			} else if key != "" {
				switch strings.ToLower(key) {
				case "action":
					newAction.Action = value
				case "label":
					newAction.Label = value
				case "url":
					newAction.URL = value
				case "method":
					newAction.Method = value
				case "body":
					newAction.Body = value
				default:
					return nil, errors.Errorf("cannot parse action: key '%s' not supported, please use JSON format instead", part)
				}
			} else {
				return nil, errors.Errorf("cannot parse action: unknown phrase '%s'", part)
			}
		}
		actions = append(actions, newAction)
	}
	return actions, nil
}
