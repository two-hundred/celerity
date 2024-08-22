package substitutions

import "github.com/two-hundred/celerity/libs/blueprint/source"

// StringOrSubstitution represents a value that can be either
// a string or a substitution represented as ${..}
// from user input.
type StringOrSubstitution struct {
	StringValue       *string
	SubstitutionValue *Substitution
	SourceMeta        *source.Meta
}

// Substitution is a representation of a placeholder provided
// with the ${..} syntax.
type Substitution struct {
	Function           *SubstitutionFunctionExpr
	Variable           *SubstitutionVariable
	ValueReference     *SubstitutionValueReference
	DataSourceProperty *SubstitutionDataSourceProperty
	ResourceProperty   *SubstitutionResourceProperty
	Child              *SubstitutionChild
	StringValue        *string
	IntValue           *int64
	FloatValue         *float64
	BoolValue          *bool
	SourceMeta         *source.Meta
}

type SubstitutionVariable struct {
	VariableName string
	SourceMeta   *source.Meta
}

type SubstitutionValueReference struct {
	ValueName  string
	Path       []*SubstitutionPathItem
	SourceMeta *source.Meta
}

type SubstitutionDataSourceProperty struct {
	DataSourceName    string
	FieldName         string
	PrimitiveArrIndex *int64
	SourceMeta        *source.Meta
}

type SubstitutionResourceProperty struct {
	ResourceName string
	Path         []*SubstitutionPathItem
	SourceMeta   *source.Meta
}

type SubstitutionPathItem struct {
	FieldName         string
	PrimitiveArrIndex *int64
}

type SubstitutionChild struct {
	ChildName  string
	Path       []*SubstitutionPathItem
	SourceMeta *source.Meta
}

type SubstitutionFunctionExpr struct {
	FunctionName SubstitutionFunctionName
	Arguments    []*SubstitutionFunctionArg
	// Path for values accessed from the function result
	// when the return value is an array or object.
	Path       []*SubstitutionPathItem
	SourceMeta *source.Meta
}

type SubstitutionFunctionArg struct {
	Name       string
	Value      *Substitution
	SourceMeta *source.Meta
}

// SubstitutionFunctionName is a type alias reserved for names
// of functions that can be called in a substitution.
type SubstitutionFunctionName string
