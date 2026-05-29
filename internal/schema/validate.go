package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/goccy/go-yaml"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"
)

// Validator compiles the config schemas once and validates YAML documents
// against them, returning *FriendlyError diagnostics on failure.
type Validator struct {
	mu       sync.Mutex
	compiled map[Kind]*jsv.Schema
}

// NewValidator returns a ready-to-use validator with an empty schema cache.
func NewValidator() *Validator {
	return &Validator{compiled: make(map[Kind]*jsv.Schema)}
}

func (v *Validator) schema(k Kind) (*jsv.Schema, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok := v.compiled[k]; ok {
		return s, nil
	}
	doc, err := document(k)
	if err != nil {
		return nil, err
	}
	c := jsv.NewCompiler()
	url := "mem:///" + k.FileName()
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("add %s schema: %w", k, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile %s schema: %w", k, err)
	}
	v.compiled[k] = s
	return s, nil
}

// ValidateBytes validates a YAML document against the schema for kind k. path is
// used only for diagnostics. A nil return means the document is structurally valid.
func (v *Validator) ValidateBytes(k Kind, path string, data []byte) error {
	s, err := v.schema(k)
	if err != nil {
		return err
	}
	inst, err := yamlToInstance(data)
	if err != nil {
		return &FriendlyError{Path: path, Messages: []string{fmt.Sprintf("could not parse YAML: %v", err)}}
	}
	if err := s.Validate(inst); err != nil {
		var ve *jsv.ValidationError
		if errors.As(err, &ve) {
			return friendlyFromValidation(k, path, ve)
		}
		return &FriendlyError{Path: path, Messages: []string{err.Error()}}
	}
	return nil
}

// yamlToInstance converts YAML bytes into the JSON data model the validator
// expects (objects as map[string]any, numbers as json.Number). We route through
// JSON so that YAML-native integers validate correctly against type: integer.
func yamlToInstance(data []byte) (any, error) {
	var generic any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	jb, err := json.Marshal(generic)
	if err != nil {
		return nil, err
	}
	return jsv.UnmarshalJSON(bytes.NewReader(jb))
}
