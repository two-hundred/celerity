package core

// ConfigDefinition contains a detailed definition (schema) of the configuration
// required for a provider or transformer plugin.
type ConfigDefinition struct {
	Fields map[string]*ConfigFieldDefinition `json:"fields"`
}

// ConfigFieldDefinition represents a field in a configuration definition
// for a provider or transformer plugin.
type ConfigFieldDefinition struct {
	Type          ScalarType     `json:"type"`
	Label         string         `json:"label"`
	Description   string         `json:"description"`
	DefaultValue  *ScalarValue   `json:"defaultValue,omitempty"`
	AllowedValues []*ScalarValue `json:"allowedValues,omitempty"`
	Examples      []*ScalarValue `json:"examples,omitempty"`
	Required      bool           `json:"required"`
}
