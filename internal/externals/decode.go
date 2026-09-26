package externals

import (
	"bytes"
	"encoding/json"
)

// These objects are edited as JSON in the desktop. Silently discarding an
// unsupported scope or mapping key would broaden what the user approved.
func decodeClosed(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
func (c *Constraint) UnmarshalJSON(data []byte) error {
	type plain Constraint
	return decodeClosed(data, (*plain)(c))
}
func (r *Request) UnmarshalJSON(data []byte) error {
	type plain Request
	return decodeClosed(data, (*plain)(r))
}
func (p *Parameter) UnmarshalJSON(data []byte) error {
	type plain Parameter
	return decodeClosed(data, (*plain)(p))
}
