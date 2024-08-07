package substitutions

import (
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/pkg/source"
	"gopkg.in/yaml.v3"
)

// DetermineYAMLSourceStartMeta is a utility function that determines the source start meta
// to use as the accurate starting point for counting lines and columns for interpolated
// substitutions in YAML documents.
//
// For "literal" style blocks, "|\s*\n" must be accounted for.
// For "folded" style blocks, ">\s*\n" must be accounted for.
func DetermineYAMLSourceStartMeta(node *yaml.Node, sourceMeta *source.Meta) *source.Meta {
	if node.Kind != yaml.ScalarNode {
		return sourceMeta
	}

	if node.Style == yaml.LiteralStyle {
		return &source.Meta{
			Line:   sourceMeta.Line + 1,
			Column: sourceMeta.Column,
		}
	}

	if node.Style == yaml.FoldedStyle {
		return &source.Meta{
			Line:   sourceMeta.Line + 1,
			Column: sourceMeta.Column,
		}
	}

	return sourceMeta
}

// ContainsSubstitution checks if a string contains a ${..} substitution.
func ContainsSubstitution(str string) bool {
	openIndex := strings.Index(str, "${")
	closeIndex := strings.Index(str, "}")
	return openIndex > -1 && closeIndex > openIndex
}
