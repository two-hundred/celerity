package core

import (
	"encoding/json"

	"github.com/two-hundred/celerity/libs/blueprint/source"
	"github.com/two-hundred/celerity/libs/blueprint/substitutions"
	"gopkg.in/yaml.v3"
)

// MappingNode provides a tree structure for user-defined
// resource specs or metadata mappings.
//
// This is used to allow creators of resource providers to define
// custom specifications while supporting ${..} placeholder substitution
// as a first class member of the primary representation of the spec.
// The initial intention was to allow the definition of custom native structs
// for resource specs, but this was abandoned in favour of a structure that will
// make placeholder substitution easier to deal with for the framework.
//
// This is also used to provide a tree structure for metadata mappings
// to facilitate substitutions at all levels of nesting in user-provided
// metadata.
type MappingNode struct {
	// Literal represents a literal value in a mapping node.
	Literal *ScalarValue
	// Fields represents a map of field names to child mapping nodes.
	Fields map[string]*MappingNode
	// Items represents a slice of child mapping nodes.
	Items []*MappingNode
	// StringWithSubstitutions is a slice of strings and substitutions
	// where substitutions are a representation of placeholders for variables,
	// resource properties, data source properties and child blueprint properties
	// or function calls contained within ${..}.
	StringWithSubstitutions *substitutions.StringOrSubstitutions
	// SourceMeta is the source metadata for the mapping node,
	// this is optional and may or may not be set depending on the context
	// and the source blueprint.
	SourceMeta *source.Meta
	// FieldsSourceMeta is a map of field names to source metadata
	// used to store the source location of fields in the original source.
	// This is optional and may or may not be set depending on the context
	// and the source blueprint.
	FieldsSourceMeta map[string]*source.Meta
}

// MarshalYAML fulfils the yaml.Marshaler interface
// to marshal a mapping node into a YAML representation.
func (m *MappingNode) MarshalYAML() (interface{}, error) {
	if m.Literal != nil {
		return m.Literal, nil
	}

	if m.StringWithSubstitutions != nil {
		return m.StringWithSubstitutions, nil
	}

	if m.Fields != nil {
		return m.Fields, nil
	}

	if m.Items != nil {
		return m.Items, nil
	}

	return nil, errMissingMappingNode(nil)
}

// UnmarshalYAML fulfils the yaml.Unmarshaler interface
// to unmarshal a YAML representation into a mapping node.
func (m *MappingNode) UnmarshalYAML(node *yaml.Node) error {
	m.SourceMeta = &source.Meta{
		Position: source.Position{
			Line:   node.Line,
			Column: node.Column,
		},
	}

	if node.Kind == yaml.ScalarNode {
		return m.parseYAMLSubstitutionsOrScalar(node)
	}

	if node.Kind == yaml.SequenceNode {
		m.Items = make([]*MappingNode, len(node.Content))
		for i, item := range node.Content {
			m.Items[i] = &MappingNode{}
			if err := m.Items[i].UnmarshalYAML(item); err != nil {
				return err
			}
		}
		return nil
	}

	if node.Kind == yaml.MappingNode {
		m.Fields = make(map[string]*MappingNode)
		m.FieldsSourceMeta = make(map[string]*source.Meta)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]

			m.Fields[key.Value] = &MappingNode{}
			if err := m.Fields[key.Value].UnmarshalYAML(value); err != nil {
				return err
			}
			m.FieldsSourceMeta[key.Value] = &source.Meta{
				Position: source.Position{
					Line:   key.Line,
					Column: key.Column,
				},
			}
		}
		return nil
	}

	return errInvalidMappingNode(node)
}

func (m *MappingNode) parseYAMLSubstitutionsOrScalar(node *yaml.Node) error {
	sourceMeta := &source.Meta{
		Position: source.Position{
			Line:   node.Line,
			Column: node.Column,
		},
		EndPosition: source.EndSourcePositionFromYAMLScalarNode(node),
	}

	isBlockStyle := node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle
	precedingCharCount := substitutions.GetYAMLNodePrecedingCharCount(node)
	sourceStartMeta := substitutions.DetermineYAMLSourceStartMeta(node, sourceMeta)
	strSubs, err := substitutions.ParseSubstitutionValues(
		"", // substitutionContext
		node.Value,
		sourceStartMeta,
		true, // outputLineInfo
		// Due to the difficulty involved in getting the precise starting column
		// of a "folded" or "literal" style block in a mapping or sequence,
		// the column number should be ignored until the difficulty of doing so changes.
		isBlockStyle, // ignoreParentColumn
		precedingCharCount,
	)
	if err != nil {
		// When substitutions are present but invalid, we must return an error to provide
		// the best possible user experience when debugging issues with a blueprint,
		// silently ignoring invalid substitutions and falling back to string literals
		// would make it harder to debug.
		return err
	} else if len(strSubs) == 0 || (len(strSubs) == 1 && strSubs[0].StringValue != nil) {
		// Parse literal value if there are no substitutions.
		m.Literal = &ScalarValue{}
		return m.Literal.UnmarshalYAML(node)
	}

	m.StringWithSubstitutions = &substitutions.StringOrSubstitutions{
		Values:     strSubs,
		SourceMeta: sourceMeta,
	}
	return nil
}

// MarshalJSON fulfils the json.Marshaler interface
// to marshal a blueprint mapping node into a JSON representation.
func (m *MappingNode) MarshalJSON() ([]byte, error) {
	if m.Literal != nil {
		return json.Marshal(m.Literal)
	}

	if m.StringWithSubstitutions != nil {
		return json.Marshal(m.StringWithSubstitutions)
	}

	if m.Fields != nil {
		return json.Marshal(m.Fields)
	}

	if m.Items != nil {
		return json.Marshal(m.Items)
	}

	return nil, errMissingMappingNode(nil)
}

// UnmarshalJSON fulfils the json.Unmarshaler interface
// to unmarshal a serialised blueprint mapping node.
func (m *MappingNode) UnmarshalJSON(data []byte) error {

	var items []*MappingNode
	if err := json.Unmarshal(data, &items); err == nil {
		m.Items = items
		return nil
	}

	var fields map[string]*MappingNode
	if err := json.Unmarshal(data, &fields); err == nil {
		m.Fields = fields
		return nil
	}

	err := m.parseJSONSubstitutionsOrScalar(data)
	if err == nil {
		return nil
	}

	return errInvalidMappingNode(nil)
}

func (m *MappingNode) parseJSONSubstitutionsOrScalar(data []byte) error {
	dataStr := string(data)
	// Remove the quotes from the string
	normalised := dataStr
	if len(dataStr) >= 2 && dataStr[0] == '"' && dataStr[len(dataStr)-1] == '"' {
		normalised = dataStr[1 : len(dataStr)-1]
	}
	strSubs, err := substitutions.ParseSubstitutionValues("", normalised, nil, false, true, 0)

	if err != nil {
		// When substitutions are present but invalid, we must return an error to provide
		// the best possible user experience when debugging issues with a blueprint,
		// silently ignoring invalid substitutions and falling back to string literals
		// would make it harder to debug.
		return err
	} else if len(strSubs) == 0 || (len(strSubs) == 1 && strSubs[0].StringValue != nil) {
		// Parse literal value if there are no substitutions.
		m.Literal = &ScalarValue{}
		return m.Literal.UnmarshalJSON(data)
	}

	m.StringWithSubstitutions = &substitutions.StringOrSubstitutions{
		Values: strSubs,
	}
	return nil
}
