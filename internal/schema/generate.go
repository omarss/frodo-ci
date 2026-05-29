// Package schema generates JSON Schemas from the Frodo CI config types and
// validates config files against them with human-friendly diagnostics.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/omarss/frodo-ci/internal/config"
)

// Kind identifies a config schema.
type Kind string

const (
	KindRoot         Kind = "root"
	KindModule       Kind = "module"
	KindStage        Kind = "stage"
	KindToolchains   Kind = "toolchains"
	KindSecurity     Kind = "security"
	KindLint         Kind = "lint"
	KindPerformance  Kind = "performance"
	KindSuppressions Kind = "suppressions" // validated, not part of the exported seven
	KindRulesets     Kind = "rulesets"     // validated, not part of the exported seven
)

// FileName returns the schema's on-disk file name.
func (k Kind) FileName() string { return string(k) + ".schema.json" }

// ExportedKinds lists the seven schemas shipped by `frodo-ci init` / exported by
// `frodo-ci schemas export`, matching the requirements.
func ExportedKinds() []Kind {
	return []Kind{
		KindRoot, KindModule, KindStage, KindToolchains,
		KindSecurity, KindLint, KindPerformance,
	}
}

// defaultCIStages and defaultCDStages bake the standard stage names into the
// static module schema so editors can validate/complete stage keys offline. The
// semantic linter re-checks stage names against the repo's actual root config.
var (
	defaultCIStages = []any{"validate", "test", "build", "package", "scan"}
	defaultCDStages = []any{"publish", "deploy", "verify"}
)

func prototype(k Kind) any {
	switch k {
	case KindRoot:
		return &config.RootConfig{}
	case KindModule:
		return &config.ModuleConfig{}
	case KindStage:
		return &config.StageFile{}
	case KindToolchains:
		return &config.Toolchains{}
	case KindSecurity:
		return &config.SecurityBaseline{}
	case KindLint:
		return &config.LintRules{}
	case KindPerformance:
		return &config.PerformanceBudgets{}
	case KindSuppressions:
		return &config.Suppressions{}
	case KindRulesets:
		return &config.Rulesets{}
	default:
		return nil
	}
}

func newReflector() *jsonschema.Reflector {
	return &jsonschema.Reflector{
		ExpandedStruct: true, // inline the root type instead of a top-level $ref
		Anonymous:      true, // omit $id so refs stay local (#/$defs/...)
		Mapper:         customMapper,
	}
}

// customMapper maps the custom scalar types onto the JSON shapes their YAML
// representation actually takes.
func customMapper(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeOf(config.Duration(0)):
		return &jsonschema.Schema{
			Type:        "string",
			Pattern:     `^([0-9]+(\.[0-9]+)?(s|m|h|d|w))+$`,
			Title:       "duration",
			Description: "Duration with units s, m, h, d, or w (e.g. 30d, 20m, 1h30m).",
		}
	case reflect.TypeOf(config.Date{}):
		return &jsonschema.Schema{Type: "string", Format: "date", Description: "Calendar date (YYYY-MM-DD)."}
	case reflect.TypeOf(config.FlexStr("")):
		// anyOf (not oneOf): an integer like 25 is also a valid number, so the
		// subschemas intentionally overlap and any single match is acceptable.
		return &jsonschema.Schema{
			AnyOf: []*jsonschema.Schema{
				{Type: "string"}, {Type: "integer"}, {Type: "number"}, {Type: "boolean"},
			},
			Description: "A scalar value, kept as text.",
		}
	case reflect.TypeOf(config.Matcher{}):
		return matcherSchema()
	default:
		return nil
	}
}

func matcherSchema() *jsonschema.Schema {
	props := orderedmap.New[string, *jsonschema.Schema]()
	props.Set("regex", &jsonschema.Schema{
		Type:        "string",
		Description: "A regular expression matched against repo-relative paths.",
	})
	obj := &jsonschema.Schema{
		Type:                 "object",
		Properties:           props,
		Required:             []string{"regex"},
		AdditionalProperties: jsonschema.FalseSchema,
	}
	return &jsonschema.Schema{
		OneOf:       []*jsonschema.Schema{{Type: "string"}, obj},
		Description: "A glob string, or a {regex: ...} object.",
	}
}

// build reflects the schema for a kind and applies kind-specific refinements.
func build(k Kind) *jsonschema.Schema {
	s := newReflector().Reflect(prototype(k))
	if k == KindModule {
		restrictStageNames(s, "ci", defaultCIStages)
		restrictStageNames(s, "cd", defaultCDStages)
	}
	return s
}

// restrictStageNames adds a propertyNames enum so unknown stage keys (e.g.
// "tests") fail validation with the allowed list, enabling did-you-mean.
func restrictStageNames(s *jsonschema.Schema, field string, stages []any) {
	if s.Properties == nil {
		return
	}
	if prop, ok := s.Properties.Get(field); ok && prop != nil {
		prop.PropertyNames = &jsonschema.Schema{Enum: stages}
	}
}

// JSON returns the indented JSON Schema document for a kind.
func JSON(k Kind) ([]byte, error) {
	return json.MarshalIndent(build(k), "", "  ")
}

// document parses the generated schema into the json model the validator wants.
func document(k Kind) (any, error) {
	b, err := JSON(k)
	if err != nil {
		return nil, fmt.Errorf("generate %s schema: %w", k, err)
	}
	doc, err := jsv.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("load %s schema: %w", k, err)
	}
	return doc, nil
}
