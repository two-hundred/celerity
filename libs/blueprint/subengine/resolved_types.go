package subengine

import (
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
)

// ResolvedValue provides a version of a value for which all ${..}
// substitutions have been applied.
type ResolvedValue struct {
	Type        *schema.ValueTypeWrapper `json:"type"`
	Value       *core.MappingNode        `json:"value"`
	Description *core.MappingNode        `json:"description,omitempty"`
	Secret      *core.ScalarValue        `json:"secret"`
}

// ResolvedInclude provides a version of a child blueprint
// include for which all ${..} substitutions have been applied.
type ResolvedInclude struct {
	Path        *core.MappingNode `json:"path"`
	Variables   *core.MappingNode `json:"variables,omitempty"`
	Metadata    *core.MappingNode `json:"metadata,omitempty"`
	Description *core.MappingNode `json:"description,omitempty"`
}

// ResolvedExport provides a version of an export
// for which all ${..} substitutions have been applied.
type ResolvedExport struct {
	Type        *schema.ExportTypeWrapper `json:"type"`
	Field       *core.ScalarValue         `json:"field"`
	Description *core.MappingNode         `json:"description,omitempty"`
}