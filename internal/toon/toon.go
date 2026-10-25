// Package toon encodes values in TOON (Token-Oriented Object Notation).
package toon

import (
	"encoding/json"

	"github.com/alpkeskin/gotoon"
)

// Encode encodes v in TOON with the same field names and omitempty rules as
// its JSON encoding. gotoon reads the raw json struct tag, so encoding a
// struct directly turned `json:"symbol_name,omitempty"` into a column named
// "symbol_name,omitempty". Going through encoding/json first avoids that.
func Encode(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return "", err
	}
	return gotoon.Encode(normalized)
}
