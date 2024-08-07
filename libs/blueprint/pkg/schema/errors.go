package schema

import (
	"fmt"
	"strings"

	bpcore "github.com/two-hundred/celerity/libs/blueprint/pkg/core"
	"github.com/two-hundred/celerity/libs/blueprint/pkg/errors"
	"github.com/two-hundred/celerity/libs/common/pkg/core"
	"gopkg.in/yaml.v3"
)

// Error represents an error due to an issue
// with the schema of a blueprint.
type Error struct {
	ReasonCode ErrorSchemaReasonCode
	Err        error
	// The line in the source blueprint file
	// where the error occurred.
	// This will be nil if the error is not related
	// to a specific line in the blueprint file
	// or the source format is JSON.
	SourceLine *int
	// The column on a line in the source blueprint file
	// where the error occurred.
	// This will be nil if the error is not related
	// to a specific line/column in the blueprint file
	// or the source format is JSON.
	SourceColumn *int
}

func (e *Error) Error() string {
	return e.Err.Error()
}

type ErrorSchemaReasonCode string

const (
	// ErrorSchemaReasonCodeInvalidVariableType is provided
	// when the reason for a blueprint schema load error is due
	// to an invalid variable type.
	ErrorSchemaReasonCodeInvalidVariableType ErrorSchemaReasonCode = "invalid_variable_type"
	// ErrorSchemaReasonCodeInvalidDataSourceFieldType is provided
	// when the reason for a blueprint schema load error is due
	// to an invalid data source exported field type.
	ErrorSchemaReasonCodeInvalidDataSourceFieldType ErrorSchemaReasonCode = "invalid_data_source_field_type"
	// ErrorSchemaReasonCodeInvalidDataSourceFilterOperator is provided
	// when the reason for a blueprint schema load error is due
	// to an invalid data source filter operator being provided.
	ErrorSchemaReasonCodeInvalidDataSourceFilterOperator ErrorSchemaReasonCode = "invalid_data_source_filter_operator"
	// ErrorSchemaReasonCodeInvalidTransformType is provided
	// when the reason for a blueprint schema load error is due to
	// an invalid transform field value being provided.
	ErrorSchemaReasonCodeInvalidTransformType ErrorSchemaReasonCode = "invalid_transform_type"
	// ErrorSchemaReasonCodeInvalidMap is provided when the reason
	// for a blueprint schema load error is due to an invalid map
	// being provided.
	ErrorSchemaReasonCodeInvalidMap ErrorSchemaReasonCode = "invalid_map"
	// ErrorSchemaReasonCodeGeneral is provided when the reason
	// for a blueprint schema load error is not specific,
	// primarily used for errors wrapped with parent scope line information.
	ErrorSchemaReasonCodeGeneral ErrorSchemaReasonCode = "general"
)

func errInvalidDataSourceFieldType(
	dataSourceFieldType DataSourceFieldType,
	line *int,
	column *int,
) error {
	return &Error{
		ReasonCode: ErrorSchemaReasonCodeInvalidDataSourceFieldType,
		Err: fmt.Errorf(
			"unsupported data source field type %s has been provided, you can choose from string, integer, float, boolean, object and array",
			dataSourceFieldType,
		),
		SourceLine:   line,
		SourceColumn: column,
	}
}

func errInvalidDataSourceFilterOperator(
	dataSourceFilterOperator DataSourceFilterOperator,
	line *int,
	column *int,
) error {
	return &Error{
		ReasonCode: ErrorSchemaReasonCodeInvalidDataSourceFilterOperator,
		Err: fmt.Errorf(
			"unsupported data source filter operator %s has been provided, you can choose from %s",
			dataSourceFilterOperator,
			strings.Join(
				core.Map(DataSourceFilterOperators, func(operator DataSourceFilterOperator, index int) string {
					return string(operator)
				}),
				",",
			),
		),
		SourceLine:   line,
		SourceColumn: column,
	}
}

func errInvalidTransformType(underlyingError error, line *int, column *int) error {
	return &Error{
		ReasonCode: ErrorSchemaReasonCodeInvalidTransformType,
		Err: fmt.Errorf(
			"unsupported type provided for spec transform, must be string or a list of strings: %s",
			underlyingError.Error(),
		),
		SourceLine:   line,
		SourceColumn: column,
	}
}

func errInvalidMap(value *yaml.Node, field string) error {
	innerError := fmt.Errorf("an invalid value has been provided for %s, expected a mapping", field)
	if value == nil {
		return &Error{
			ReasonCode: ErrorSchemaReasonCodeInvalidMap,
			Err:        innerError,
		}
	}

	return &Error{
		ReasonCode:   ErrorSchemaReasonCodeInvalidMap,
		Err:          innerError,
		SourceLine:   &value.Line,
		SourceColumn: &value.Column,
	}
}

func errInvalidGeneralMap(value *yaml.Node) error {
	innerError := fmt.Errorf("an invalid value has been provided, expected a mapping")
	if value == nil {
		return &Error{
			ReasonCode: ErrorSchemaReasonCodeInvalidMap,
			Err:        innerError,
		}
	}

	return &Error{
		ReasonCode:   ErrorSchemaReasonCodeInvalidMap,
		Err:          innerError,
		SourceLine:   &value.Line,
		SourceColumn: &value.Column,
	}
}

func wrapErrorWithLineInfo(underlyingError error, parent *yaml.Node) error {
	if _, isSchemaError := underlyingError.(*Error); isSchemaError {
		return underlyingError
	}

	if _, isCoreError := underlyingError.(*bpcore.Error); isCoreError {
		return underlyingError
	}

	if _, isLoadError := underlyingError.(*errors.LoadError); isLoadError {
		return underlyingError
	}

	return &Error{
		ReasonCode:   ErrorSchemaReasonCodeGeneral,
		Err:          underlyingError,
		SourceLine:   &parent.Line,
		SourceColumn: &parent.Column,
	}
}
