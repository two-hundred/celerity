package substitutions

import (
	"fmt"

	"github.com/two-hundred/celerity/libs/blueprint/jsonutils"
	"github.com/two-hundred/celerity/libs/blueprint/source"
	"gopkg.in/yaml.v3"
)

// StringOrSubstitutions represents a value
// that represents a string interpolated with substitutions.
type StringOrSubstitutions struct {
	Values []*StringOrSubstitution
	// SourceMeta is the source metadata for the string or substitutions,
	// this is optional and may or may not be set depending on the context
	// and the source blueprint.
	SourceMeta *source.Meta
}

// MarshalYAML fulfils the yaml.Marshaler interface
// to marshal a blueprint string or substitutions value
// into a string representation.
func (s *StringOrSubstitutions) MarshalYAML() (interface{}, error) {
	// During serialisation, there is no way of knowing the context
	// (i.e. the key or field name) in which the substitutions are being used.
	// This is why an empty string is passed as the substitution context.
	return SubstitutionsToString("", s)
}

// UnmarshalYAML fulfils the yaml.Unmarshaler interface
// to unmarshal a string that could contain interpolated
// references.
func (s *StringOrSubstitutions) UnmarshalYAML(node *yaml.Node) error {
	s.SourceMeta = &source.Meta{
		Line:   node.Line,
		Column: node.Column,
	}

	isBlockStyle := node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle
	sourceStartMeta := DetermineYAMLSourceStartMeta(node, s.SourceMeta)
	// During deserialisation, there is no way of knowing the context
	// (i.e. the key or field name) in which the substitutions are being used.
	// This is why an empty string is passed as the substitution context.
	parsedValues, err := ParseSubstitutionValues(
		"", // substitutionContext
		node.Value,
		sourceStartMeta,
		true, // outputLineInfo
		// Due to the difficulty involved in getting the precise starting column
		// of a "folded" or "literal" style block in a mapping or sequence,
		// the column number should be ignored until the difficulty of doing so changes.
		isBlockStyle, // ignoreParentColumn
	)
	if err != nil {
		return err
	}
	s.Values = parsedValues
	return nil
}

// MarshalJSON fulfils the json.Marshaler interface
// to marshal a blueprint string or substitutions value
// into a string representation.
func (v *StringOrSubstitutions) MarshalJSON() ([]byte, error) {
	str, err := SubstitutionsToString("", v)
	if err != nil {
		return nil, err
	}
	escaped := jsonutils.EscapeJSONString(str)
	return []byte(fmt.Sprintf("\"%s\"", escaped)), nil
}

// UnmarshalJSON fulfils the json.Unmarshaler interface
// to unmarshal a string that could contain interpolated
// references.
func (s *StringOrSubstitutions) UnmarshalJSON(data []byte) error {
	dataStr := string(data)
	// Remove the quotes from the string
	if len(dataStr) < 2 || dataStr[0] != '"' || dataStr[len(dataStr)-1] != '"' {
		return errSubstitutions(
			"",
			[]error{fmt.Errorf("invalid string value: %s", dataStr)},
			nil,
			nil,
		)
	}
	quotesStripped := dataStr[1 : len(dataStr)-1]
	// Ensure that all JSON special characters are unescaped, otherwise
	// the parser will fail for substitutions that contains characters that are special
	// in JSON like '"'.
	unescaped := jsonutils.UnescapeJSONString(quotesStripped)

	// During deserialisation, there is no way of knowing the context
	// (i.e. the key or field name) in which the substitutions are being used.
	// This is why an empty string is passed as the substitution context.
	parsedValues, err := ParseSubstitutionValues("", unescaped, nil, false, true)
	if err != nil {
		return err
	}
	s.Values = parsedValues
	return nil
}
