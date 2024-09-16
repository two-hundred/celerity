package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/source"
	"github.com/two-hundred/celerity/libs/common/core"
	"gopkg.in/yaml.v3"
)

// TransformValueWrapper holds one or more transforms
// to be applied to a specification.
// This allows for users to provide the transform field in a spec
// as a string or as a list of strings.
type TransformValueWrapper struct {
	Values []string
	// A list of source meta information for each transform value
	// that if populated, will be in the same order as the transform values.
	SourceMeta []*source.Meta
}

func (t *TransformValueWrapper) MarshalYAML() (interface{}, error) {
	// Always marshal as a slice.
	return t.Values, nil
}

func (t *TransformValueWrapper) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		t.Values = []string{value.Value}
		t.SourceMeta = []*source.Meta{
			{
				Position: source.Position{
					Line:   value.Line,
					Column: value.Column,
				},
				EndPosition: source.EndSourcePositionFromYAMLScalarNode(value),
			},
		}
		return nil
	}

	if value.Kind == yaml.SequenceNode {
		values, positions, err := collectStringNodeValues(value.Content)
		if err != nil {
			return err
		}
		t.Values = values
		t.SourceMeta = positions
		return nil
	}

	return errInvalidTransformType(
		fmt.Errorf("unexpected yaml node for transform: %s", yamlKindMappings[value.Kind]),
		&value.Line,
		&value.Column,
	)
}

func (t *TransformValueWrapper) MarshalJSON() ([]byte, error) {
	// Always marshal as a slice.
	return json.Marshal(t.Values)
}

func (t *TransformValueWrapper) UnmarshalJSON(data []byte) error {
	transformValues := []string{}
	// Try to parse a slice, then fall back to a single string.
	// There is no better way to know with the built-in JSON library,
	// yes there are more efficient checks you can do by simply looking
	// at the characters in the string but they will not be as reliable
	// as unmarshalling.
	err := json.Unmarshal(data, &transformValues)
	if err == nil {
		t.Values = transformValues
		return nil
	}

	var transformValue string
	err = json.Unmarshal(data, &transformValue)
	if err != nil {
		return errInvalidTransformType(
			fmt.Errorf("unexpected value provided for transform in json: %s", err.Error()),
			nil,
			nil,
		)
	}
	t.Values = []string{transformValue}
	return nil
}

func collectStringNodeValues(nodes []*yaml.Node) ([]string, []*source.Meta, error) {
	values := []string{}
	sourceMeta := []*source.Meta{}
	// For at least 99% of the cases it will be trivial to go through
	// the entire list of transform value nodes and identify any invalid
	// values. This is much better for users of the spec too!
	nonScalarNodeKinds := []yaml.Kind{}
	firstNonScalarIndex := -1
	for i, node := range nodes {
		if node.Kind != yaml.ScalarNode {
			nonScalarNodeKinds = append(nonScalarNodeKinds, node.Kind)
			if firstNonScalarIndex == -1 {
				firstNonScalarIndex = i
			}
		} else {
			values = append(values, node.Value)
			sourceMeta = append(sourceMeta, &source.Meta{
				Position: source.Position{
					Line:   node.Line,
					Column: node.Column,
				},
				EndPosition: source.EndSourcePositionFromYAMLScalarNode(node),
			})
		}
	}

	if len(nonScalarNodeKinds) > 0 {
		return nil, nil, errInvalidTransformType(
			fmt.Errorf(
				"unexpected yaml nodes in transform list, only scalars are supported: %s",
				formatYamlNodeKindsForError(nonScalarNodeKinds),
			),
			// Take the position of the first non-scalar node,
			// the error message will be detailed enough for the user to figure out
			// which transforms in the list are invalid.
			&nodes[firstNonScalarIndex].Line,
			&nodes[firstNonScalarIndex].Column,
		)
	}

	return values, sourceMeta, nil
}

func formatYamlNodeKindsForError(nodeKinds []yaml.Kind) string {
	return strings.Join(
		core.Map(nodeKinds, func(kind yaml.Kind, index int) string {
			return fmt.Sprintf("%d:%s", index, yamlKindMappings[kind])
		}),
		",",
	)
}

var yamlKindMappings = map[yaml.Kind]string{
	yaml.AliasNode:    "alias",
	yaml.DocumentNode: "document",
	yaml.ScalarNode:   "scalar",
	yaml.MappingNode:  "mapping",
	yaml.SequenceNode: "sequence",
}
