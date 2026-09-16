package cdxp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// object is a JSON object that remembers the key order of the source file. The
// shell version got that order for free from jq and used it for -c flag order
// and for show output, so preserving it keeps the two implementations
// byte-identical.
type object struct {
	keys []string
	vals map[string]any
}

// Get returns the value stored under k.
func (o *object) Get(k string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[k]
	return v, ok
}

// At returns the value stored under k, or nil.
func (o *object) At(k string) any {
	v, _ := o.Get(k)
	return v
}

// Keys lists the keys in source order.
func (o *object) Keys() []string {
	if o == nil {
		return nil
	}
	return o.keys
}

// Len counts the keys.
func (o *object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Objects are also stored as ordered objects; slices stay plain []any.
func newObject() *object { return &object{vals: map[string]any{}} }

// parseJSON decodes one JSON value, keeping object key order and the literal
// spelling of numbers.
func parseJSON(r io.Reader) (any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return decodeFromToken(dec, tok)
}

func decodeFromToken(dec *json.Decoder, tok json.Token) (any, error) {
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		obj := newObject()
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := kt.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			if _, dup := obj.vals[key]; !dup {
				obj.keys = append(obj.keys, key)
			}
			obj.vals[key] = val
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return obj, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", delim)
}

// encodeJSON renders a decoded value as compact JSON, matching jq's tojson.
func encodeJSON(v any) string {
	var b strings.Builder
	writeJSON(&b, v)
	return b.String()
}

func writeJSON(b *strings.Builder, v any) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case string:
		b.WriteString(jsonQuote(t))
	case json.Number:
		b.WriteString(t.String())
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSON(b, e)
		}
		b.WriteByte(']')
	case *object:
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(jsonQuote(k))
			b.WriteByte(':')
			writeJSON(b, t.vals[k])
		}
		b.WriteByte('}')
	default:
		b.WriteString(jsonQuote(fmt.Sprint(v)))
	}
}
