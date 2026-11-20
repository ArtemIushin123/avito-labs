package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/requestbody"
)

func requiredObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	m, err := requestbody.Object(raw, keys...)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if m[key] == nil || bytes.Equal(m[key], []byte("null")) {
			return nil, fmt.Errorf("%s is required", key)
		}
	}
	return m, nil
}

func parsePosition(body []byte) (api.PositionData, error) {
	var data api.PositionData
	fields, err := requiredObject(body, "coordinates", "recorded_at")
	if err != nil {
		return data, err
	}
	if _, err := requiredObject(fields["coordinates"], "latitude", "longitude"); err != nil {
		return data, fmt.Errorf("coordinates: %w", err)
	}
	if err := json.Unmarshal(fields["coordinates"], &data.Coordinates); err != nil {
		return data, errors.New("coordinates must contain numeric latitude and longitude")
	}
	if err := json.Unmarshal(fields["recorded_at"], &data.RecordedAt); err != nil {
		return data, errors.New("recorded_at must be an RFC3339 timestamp")
	}
	return data, nil
}
