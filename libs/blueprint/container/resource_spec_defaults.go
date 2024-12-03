package container

import (
	"context"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
)

// PopulateResourceSpecDefaults populates the default values for missing values
// in each resource spec in the given blueprint.
func PopulateResourceSpecDefaults(
	ctx context.Context,
	blueprint *schema.Blueprint,
	params core.BlueprintParams,
	resourceRegistry resourcehelpers.Registry,
) (*schema.Blueprint, error) {
	if blueprint.Resources == nil {
		return blueprint, nil
	}

	newResourceMap := &schema.ResourceMap{
		Values: map[string]*schema.Resource{},
	}
	for resourceName, resource := range blueprint.Resources.Values {
		if resource.Type != nil {
			specDefOutput, err := resourceRegistry.GetSpecDefinition(
				ctx,
				resource.Type.Value,
				&provider.ResourceGetSpecDefinitionInput{
					Params: params,
				},
			)
			if err != nil {
				return nil, err
			}

			if specDefOutput.SpecDefinition == nil ||
				specDefOutput.SpecDefinition.Schema == nil {
				newResourceMap.Values[resourceName] = resource
			} else {
				newSpec := populateDefaultValues(
					resource.Spec,
					specDefOutput.SpecDefinition.Schema,
					/* depth */ 0,
				)
				newResourceMap.Values[resourceName] = &schema.Resource{
					Type: resource.Type,
					Spec: newSpec,
				}
			}
		}
	}

	return &schema.Blueprint{
		Version:     blueprint.Version,
		Transform:   blueprint.Transform,
		Variables:   blueprint.Variables,
		Values:      blueprint.Values,
		Include:     blueprint.Include,
		Resources:   newResourceMap,
		DataSources: blueprint.DataSources,
		Exports:     blueprint.Exports,
		Metadata:    blueprint.Metadata,
	}, nil
}

func populateDefaultValues(
	specValue *core.MappingNode,
	definition *provider.ResourceDefinitionsSchema,
	depth int,
) *core.MappingNode {
	if depth > validation.MappingNodeMaxTraverseDepth {
		return specValue
	}

	if core.IsNilMappingNode(specValue) &&
		definition.Default != nil &&
		!definition.Computed &&
		// Nullable values should not be populated with default values when they are nil,
		// a field being nullable means that it can be explicitly set to null (nil in Go).
		!definition.Nullable {
		// For unions, only a default value on the union schema definition will be considered,
		// and not on the individual union types.
		// This is because the resource definition schema does not provide a way to select
		// which type in the union should be used when a default value is provided.
		return definition.Default
	}

	if definition.Computed {
		// Do not try to populate defaults for any computed values.
		return specValue
	}

	if core.IsObjectMappingNode(specValue) &&
		definition.Type == provider.ResourceDefinitionsSchemaTypeObject &&
		definition.Attributes != nil {

		return populateDefaultsInObject(specValue, definition, depth)
	}

	if core.IsObjectMappingNode(specValue) &&
		definition.Type == provider.ResourceDefinitionsSchemaTypeMap &&
		definition.MapValues != nil {

		return populateDefaultsInMapValues(specValue, definition, depth)
	}

	if core.IsArrayMappingNode(specValue) &&
		definition.Type == provider.ResourceDefinitionsSchemaTypeArray &&
		definition.Items != nil {

		return populateDefaultsInArrayItems(specValue, definition, depth)
	}

	if definition.Type == provider.ResourceDefinitionsSchemaTypeUnion {

		return populateDefaultsInUnion(specValue, definition, depth)
	}

	return specValue
}

func populateDefaultsInUnion(
	specValue *core.MappingNode,
	definition *provider.ResourceDefinitionsSchema,
	depth int,
) *core.MappingNode {
	if core.IsNilMappingNode(specValue) {
		// At this point, we've established that the current spec definition
		// will not have a default value and due to there being no meaningful way
		// to decide which type in the union should be used when a default value is provided,
		// we will not populate defaults for the union.
		return nil
	}

	if core.IsObjectMappingNode(specValue) {
		matchInfo := checkMappingNodeTypesForFields(specValue.Fields, nil, definition)
		if matchInfo.schema != nil {
			return populateDefaultValues(specValue, matchInfo.schema, depth)
		}
		// If we can't match against an object or map schema in the union,
		// we will not populate defaults for the union.
		return specValue
	}

	if core.IsArrayMappingNode(specValue) {
		// This does not guarantee selection of the correct schema in a union with
		// multiple array definitions.
		// It is best to advise provider plugin developers to avoid using unions with
		// multiple array definitions.
		arraySchema := getArraySchema(definition.OneOf)
		if arraySchema != nil {
			return populateDefaultValues(specValue, arraySchema, depth)
		}
		// If we can't match against an array schema in the union,
		// we will not populate defaults for the union.
		return specValue
	}

	return specValue
}

func populateDefaultsInObject(
	specValue *core.MappingNode,
	definition *provider.ResourceDefinitionsSchema,
	depth int,
) *core.MappingNode {
	newSpecValue := &core.MappingNode{
		Fields: map[string]*core.MappingNode{},
	}
	for key, attributeDefinition := range definition.Attributes {
		newSpecValue.Fields[key] = populateDefaultValues(
			specValue.Fields[key],
			attributeDefinition,
			depth+1,
		)
	}
	return newSpecValue
}

func populateDefaultsInMapValues(
	specValue *core.MappingNode,
	definition *provider.ResourceDefinitionsSchema,
	depth int,
) *core.MappingNode {
	newSpecMapValue := &core.MappingNode{
		Fields: map[string]*core.MappingNode{},
	}
	for mapKey, mapValue := range specValue.Fields {
		// Only populate defaults within the value in the map
		// and not for the entire value in a key/value pair.
		// Providing defaults for a nil map value for an explicitly set key
		// in most cases will be seen as unexpected behaviour.
		if mapValue != nil {
			newSpecMapValue.Fields[mapKey] = populateDefaultValues(
				specValue.Fields[mapKey],
				definition.MapValues,
				depth+1,
			)
		}
	}
	return newSpecMapValue
}

func populateDefaultsInArrayItems(
	specValue *core.MappingNode,
	definition *provider.ResourceDefinitionsSchema,
	depth int,
) *core.MappingNode {
	newSpecValue := &core.MappingNode{
		Items: []*core.MappingNode{},
	}
	for _, itemValue := range specValue.Items {
		// Only populate defaults within the value in the array
		// and not for the entire element in the array.
		// Providing defaults for an array element explicitly set to nil
		// in most cases will be seen as unexpected behaviour.
		if itemValue != nil {
			newSpecValue.Items = append(
				newSpecValue.Items,
				populateDefaultValues(
					itemValue,
					definition.Items,
					depth+1,
				),
			)
		}
	}
	return newSpecValue
}