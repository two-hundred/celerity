package schema

import "github.com/two-hundred/celerity/libs/blueprint/pkg/core"

// Blueprint provides the type for a blueprint
// specification loaded into memory.
type Blueprint struct {
	Version     string                 `yaml:"version" json:"version"`
	Transform   *TransformValueWrapper `yaml:"transform" json:"transform"`
	Variables   map[string]*Variable   `yaml:"variables" json:"variables"`
	Include     map[string]*Include    `yaml:"include" json:"include"`
	Resources   map[string]*Resource   `yaml:"resources" json:"resources"`
	DataSources map[string]*DataSource `yaml:"datasources" json:"datasources"`
	Exports     map[string]*Export     `yaml:"exports" json:"exports"`
	Metadata    *core.MappingNode      `yaml:"metadata" json:"metadata"`
}
