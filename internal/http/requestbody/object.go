package requestbody

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

func Object(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, errors.New("must be a JSON object")
		}
		return nil, errors.New("malformed JSON")
	}
	for k := range m {
		if !slices.Contains(allowed, k) {
			return nil, fmt.Errorf("unknown field %q", k)
		}
	}
	return m, nil
}
