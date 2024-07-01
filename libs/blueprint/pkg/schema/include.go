package schema

import (
	"github.com/two-hundred/celerity/libs/blueprint/pkg/core"
	"github.com/two-hundred/celerity/libs/blueprint/pkg/source"
	"github.com/two-hundred/celerity/libs/blueprint/pkg/substitutions"
	"gopkg.in/yaml.v3"
)

// Include represents a child blueprint
// include in the specification.
// This provides a method of creating modular blueprints
// that is native to the spec and doesn't require
// a third party plugin to implement. (e.g. a celerity/blueprint resource type)
type Include struct {
	// The path to the child blueprint on a local or remote file system.
	Path *substitutions.StringOrSubstitutions `yaml:"path" json:"path"`
	// The variables to pass down to the child blueprint.
	Variables *core.MappingNode `yaml:"variables" json:"variables"`
	// Extra metadata to be used by include resolver plugins.
	// An example of this could be the use of fields that provide information
	// about a remote location to download the child blueprint from such as
	// an AWS S3 bucket.
	Metadata    *core.MappingNode                    `yaml:"metadata" json:"metadata"`
	Description *substitutions.StringOrSubstitutions `yaml:"description" json:"description"`
	SourceMeta  *source.Meta                         `yaml:"-" json:"-"`
}

func (t *Include) UnmarshalYAML(value *yaml.Node) error {
	t.SourceMeta = &source.Meta{
		Line:   value.Line,
		Column: value.Column,
	}

	type includeAlias Include
	var alias includeAlias
	if err := value.Decode(&alias); err != nil {
		return wrapErrorWithLineInfo(err, value)
	}

	t.Path = alias.Path
	t.Variables = alias.Variables
	t.Metadata = alias.Metadata
	t.Description = alias.Description

	return nil
}
