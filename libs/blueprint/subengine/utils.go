package subengine

import (
	"fmt"
	"math"

	bpcore "github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/substitutions"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
	"github.com/two-hundred/celerity/libs/common/core"
)

type resolveContext struct {
	rootElementName        string
	rootElementProperty    string
	currentElementName     string
	currentElementProperty string
	disallowedElementTypes []string
	resolveFor             ResolveForStage
	partiallyResolved      interface{}
}

func resolveContextFromParent(
	currentElementProperty string,
	parentCtx *resolveContext,
) *resolveContext {
	return &resolveContext{
		rootElementName:        parentCtx.rootElementName,
		rootElementProperty:    getRootElemProperty(parentCtx, currentElementProperty),
		currentElementName:     parentCtx.currentElementName,
		currentElementProperty: currentElementProperty,
		disallowedElementTypes: parentCtx.disallowedElementTypes,
		resolveFor:             parentCtx.resolveFor,
		partiallyResolved:      parentCtx.partiallyResolved,
	}
}

func resolveContextForCurrentElement(
	currentElement string,
	parentCtx *resolveContext,
) *resolveContext {
	return &resolveContext{
		rootElementName:        parentCtx.rootElementName,
		rootElementProperty:    getRootElemProperty(parentCtx, parentCtx.currentElementProperty),
		currentElementName:     currentElement,
		currentElementProperty: "",
		disallowedElementTypes: parentCtx.disallowedElementTypes,
		resolveFor:             parentCtx.resolveFor,
		partiallyResolved:      parentCtx.partiallyResolved,
	}
}

func createEmptyArgError(
	elementName string,
	functionName string,
	arg *substitutions.SubstitutionFunctionArg,
	index int,
) error {
	if arg.Name != "" {
		return errEmptyNamedFunctionArgument(elementName, functionName, arg.Name)
	}

	return errEmptyPositionalFunctionArgument(elementName, functionName, index)
}

func selectResourceForProperty(
	parentElementName string,
	prop *substitutions.SubstitutionResourceProperty,
	resources []*provider.ResolvedResource,
) (*bpcore.MappingNode, error) {
	current := resources[0].Spec
	if prop.ResourceEachTemplateIndex != nil {
		if len(resources) <= int(*prop.ResourceEachTemplateIndex) {
			return nil, errResourceEachIndexOutOfBounds(
				parentElementName,
				prop.ResourceName,
				int(*prop.ResourceEachTemplateIndex),
			)
		}

		current = resources[*prop.ResourceEachTemplateIndex].Spec
	}

	return current, nil
}

func getVariable(
	variableName string,
	schema *schema.Blueprint,
) *schema.Variable {
	if schema.Variables == nil {
		return nil
	}

	return schema.Variables.Values[variableName]
}

func getValue(
	valueName string,
	schema *schema.Blueprint,
) *schema.Value {
	if schema.Values == nil {
		return nil
	}

	return schema.Values.Values[valueName]
}

func getDataSource(
	valueName string,
	schema *schema.Blueprint,
) *schema.DataSource {
	if schema.DataSources == nil {
		return nil
	}

	return schema.DataSources.Values[valueName]
}

func isStringLiteral(s *substitutions.StringOrSubstitutions) bool {
	return len(s.Values) == 1 && s.Values[0].StringValue != nil
}

func resolvedValueToString(
	value *bpcore.MappingNode,
) (string, error) {
	if value.Literal == nil {
		return "", fmt.Errorf("only literal values can be converted to a string")
	}

	if value.Literal.StringValue != nil {
		return *value.Literal.StringValue, nil
	}

	if value.Literal.IntValue != nil {
		return fmt.Sprintf("%d", *value.Literal.IntValue), nil
	}

	if value.Literal.FloatValue != nil {
		return fmt.Sprintf("%f", *value.Literal.FloatValue), nil
	}

	if value.Literal.BoolValue != nil {
		return fmt.Sprintf("%t", *value.Literal.BoolValue), nil
	}

	return "", fmt.Errorf("expected a scalar string, int, float or bool value")
}

func mappingNodeIsArray(node *bpcore.MappingNode) bool {
	return node.Items != nil
}

func transformValueForFunctionCall(value *resolvedFunctionCallValue, _ int) any {
	if value.value != nil {
		return MappingNodeToGoValue(value.value)
	}

	return value.function
}

func getRootElemProperty(resolveCtx *resolveContext, fallbackProperty string) string {
	if resolveCtx.rootElementProperty != "" {
		return resolveCtx.rootElementProperty
	}

	return fallbackProperty
}

func handleResolveError(err error, resolveOnDeploy *[]string) error {
	if err == nil {
		return nil
	}

	if resolveOnDeployErr, ok := err.(*resolveOnDeployError); ok {
		*resolveOnDeploy = append(*resolveOnDeploy, resolveOnDeployErr.propertyPath)
		return nil
	}

	if resolveOnDeployErrs, ok := err.(*resolveOnDeployErrors); ok {
		*resolveOnDeploy = append(
			*resolveOnDeploy,
			core.Map(resolveOnDeployErrs.errors, func(err *resolveOnDeployError, _ int) string {
				return err.propertyPath
			})...,
		)
		return nil
	}

	return err
}

func handleCollectResolveError(err error, resolveOnDeployErrs *[]*resolveOnDeployError) error {
	if err == nil {
		return nil
	}

	if resolveOnDeployErr, ok := err.(*resolveOnDeployError); ok {
		*resolveOnDeployErrs = append(*resolveOnDeployErrs, resolveOnDeployErr)
		return nil
	}

	if multipleErrs, ok := err.(*resolveOnDeployErrors); ok {
		*resolveOnDeployErrs = append(
			*resolveOnDeployErrs,
			multipleErrs.errors...,
		)
		return nil
	}

	return err
}

func getResourceSpecPropertyDefinition(
	specDefinition *provider.ResourceSpecDefinition,
	property *substitutions.SubstitutionResourceProperty,
	resourceType string,
	resolveCtx *resolveContext,
) (*provider.ResourceDefinitionsSchema, error) {
	finalProperty, err := getFinalResourceSpecProperty(
		property,
		specDefinition,
		resourceType,
		resolveCtx,
	)
	if err != nil {
		return nil, err
	}

	current := specDefinition.Schema
	pathExists := true
	i := 1
	for pathExists && current != nil && i < len(finalProperty.Path) {
		pathItem := finalProperty.Path[i]
		if pathItem.FieldName != "" &&
			current.Type == provider.ResourceDefinitionsSchemaTypeObject &&
			current.Attributes != nil {
			current = current.Attributes[pathItem.FieldName]
		} else if pathItem.FieldName != "" &&
			current.Type == provider.ResourceDefinitionsSchemaTypeMap &&
			current.MapValues != nil {
			current = current.MapValues
		} else if pathItem.ArrayIndex != nil &&
			current.Type == provider.ResourceDefinitionsSchemaTypeArray &&
			current.Items != nil {
			current = current.Items
		} else {
			pathExists = false
		}

		i += 1
	}

	if !pathExists || current == nil {
		return nil, errInvalidResourceSpecProperty(
			resolveCtx.currentElementName,
			finalProperty,
			resourceType,
		)
	}

	return current, nil
}

func getFinalResourceSpecProperty(
	property *substitutions.SubstitutionResourceProperty,
	specDefinition *provider.ResourceSpecDefinition,
	resourceType string,
	resolveCtx *resolveContext,
) (*substitutions.SubstitutionResourceProperty, error) {
	if len(property.Path) == 0 {
		idField := specDefinition.IDField
		if idField == "" {
			return nil, errResourceSpecMissingIDField(
				resolveCtx.currentElementName,
				property.ResourceName,
				resourceType,
			)
		}

		return &substitutions.SubstitutionResourceProperty{
			ResourceName: property.ResourceName,
			Path: []*substitutions.SubstitutionPathItem{{
				FieldName: idField,
			}},
		}, nil
	}

	return property, nil
}

func getFinalResourceName(property *substitutions.SubstitutionResourceProperty) string {
	if property.ResourceEachTemplateIndex == nil {
		return property.ResourceName
	}

	return bpcore.ExpandedResourceName(
		property.ResourceName,
		int(*property.ResourceEachTemplateIndex),
	)
}

func getResourceSpecPropertyValue(
	resolvedResource *provider.ResolvedResource,
	property *substitutions.SubstitutionResourceProperty,
	resolveCtx *resolveContext,
) (*bpcore.MappingNode, error) {
	return getResourcePropertyValueFromMappingNode(
		resolvedResource.Spec,
		property.Path,
		property,
		resolveCtx,
		/* offset of mapping node in property path */ 1,
		errMissingResourceSpecProperty,
	)
}

func getResourceMetadataPropertyValue(
	resolvedResource *provider.ResolvedResource,
	property *substitutions.SubstitutionResourceProperty,
	resolveCtx *resolveContext,
) (*bpcore.MappingNode, error) {
	if resolvedResource.Metadata == nil {
		return nil, errResourceMetadataNotSet(
			resolveCtx.currentElementName,
			property.ResourceName,
		)
	}

	// 0 is "metadata".
	metadataProperty := property.Path[1].FieldName

	// Match for "metadata.labels[<key>]".
	if metadataProperty == "labels" && len(property.Path) == 3 {
		label := getValueFromStringMap(property.Path[2].FieldName, resolvedResource.Metadata.Labels)
		if label == "" {
			return nil, errMissingResourceMetadataProperty(
				resolveCtx.currentElementName,
				property,
				/* mappingNodeStartsAfter */ 2,
				/* depth */ 1,
				/* maxDepth */ 1,
			)
		}

		return &bpcore.MappingNode{
			Literal: &bpcore.ScalarValue{
				StringValue: &label,
			},
		}, nil
	}

	// Match for "metadata.annotations[<key>]".
	if metadataProperty == "annotations" && len(property.Path) == 3 {
		annotation := getValueFromMap(
			property.Path[2].FieldName,
			resolvedResource.Metadata.Annotations,
		)
		if annotation == nil {
			return nil, errMissingResourceMetadataProperty(
				resolveCtx.currentElementName,
				property,
				/* mappingNodeStartsAfter */ 2,
				/* depth */ 1,
				/* maxDepth */ 1,
			)
		}

		return annotation, nil
	}

	// Match for "metadata.custom.*".
	if metadataProperty == "custom" && len(property.Path) > 2 {
		return getResourcePropertyValueFromMappingNode(
			resolvedResource.Metadata.Custom,
			property.Path[2:],
			property,
			resolveCtx,
			/* offset of mapping node in property path */ 2,
			errMissingResourceMetadataProperty,
		)
	}

	// Match for "metadata.displayName".
	if metadataProperty == "displayName" && len(property.Path) == 2 {
		return resolvedResource.Metadata.DisplayName, nil
	}

	return nil, errInvalidResourceMetadataProperty(
		resolveCtx.currentElementName,
		property,
	)
}

func getResourcePropertyValueFromMappingNode(
	custom *bpcore.MappingNode,
	path []*substitutions.SubstitutionPathItem,
	property *substitutions.SubstitutionResourceProperty,
	resolveCtx *resolveContext,
	mappingNodeStartsAfter int,
	errFunc func(
		string,
		*substitutions.SubstitutionResourceProperty,
		int,
		int,
		int,
	) error,
) (*bpcore.MappingNode, error) {
	return getPathValueFromMappingNode(
		custom,
		path,
		property,
		resolveCtx,
		mappingNodeStartsAfter,
		errFunc,
	)
}

func getChildExportProperty(
	exportData *bpcore.MappingNode,
	property *substitutions.SubstitutionChild,
	resolveCtx *resolveContext,
) (*bpcore.MappingNode, error) {
	return getPathValueFromMappingNode(
		exportData,
		property.Path,
		property,
		resolveCtx,
		/* offset of mapping node in property path (children.<child>.<exportedField>.*) */ 3,
		errMissingChildExportProperty,
	)
}

func getPathValueFromMappingNode[Prop any](
	node *bpcore.MappingNode,
	path []*substitutions.SubstitutionPathItem,
	property Prop,
	resolveCtx *resolveContext,
	mappingNodeStartsAfter int,
	errFunc func(
		string,
		Prop,
		int,
		int,
		int,
	) error,
) (*bpcore.MappingNode, error) {
	current := node
	pathExists := true
	i := 0
	maxDepth := int(math.Min(validation.MappingNodeMaxTraverseDepth, float64(len(path))))
	for pathExists && current != nil && i < maxDepth {
		pathItem := path[i]
		if pathItem.FieldName != "" && current.Fields != nil {
			current = current.Fields[pathItem.FieldName]
		} else if pathItem.ArrayIndex != nil && current.Items != nil {
			current = current.Items[*pathItem.ArrayIndex]
		} else if bpcore.IsNilMappingNode(current) {
			pathExists = false
		}

		i += 1
	}

	if !pathExists || current == nil {
		return nil, errFunc(
			resolveCtx.currentElementName,
			property,
			mappingNodeStartsAfter,
			i,
			validation.MappingNodeMaxTraverseDepth,
		)
	}

	return current, nil
}

func getResourceSpecPropertyFromState(
	resourceState *state.ResourceState,
	property *substitutions.SubstitutionResourceProperty,
	resolveCtx *resolveContext,
) (*bpcore.MappingNode, error) {
	return getResourcePropertyValueFromMappingNode(
		resourceState.ResourceSpecData,
		property.Path,
		property,
		resolveCtx,
		/* offset of mapping node in property path */ 1,
		errMissingResourceSpecProperty,
	)
}

func filterOutResolvedAnnotations(
	resolvedAnnotations *bpcore.MappingNode,
	inputAnnotations *schema.StringOrSubstitutionsMap,
) *schema.StringOrSubstitutionsMap {
	if resolvedAnnotations == nil ||
		resolvedAnnotations.Fields == nil {
		return inputAnnotations
	}

	filteredAnnotations := &schema.StringOrSubstitutionsMap{
		Values: make(map[string]*substitutions.StringOrSubstitutions),
	}

	for key, value := range inputAnnotations.Values {
		if _, ok := resolvedAnnotations.Fields[key]; !ok {
			filteredAnnotations.Values[key] = value
		}
	}

	return filteredAnnotations
}

func getPartiallyResolvedResourceSpec(
	resolvedContext *resolveContext,
) *bpcore.MappingNode {
	if resolvedContext.partiallyResolved == nil {
		return nil
	}

	resource := resolvedContext.partiallyResolved.(*provider.ResolvedResource)
	if resource == nil {
		return nil
	}

	return resource.Spec
}

func getPartiallyResolvedResourceCustomMetadata(
	resolvedContext *resolveContext,
) *bpcore.MappingNode {
	if resolvedContext.partiallyResolved == nil {
		return nil
	}

	resource := resolvedContext.partiallyResolved.(*provider.ResolvedResource)
	if resource == nil {
		return nil
	}

	return resource.Metadata.Custom
}

func getPartiallyResolvedDataSourceCustomMetadata(
	resolvedContext *resolveContext,
) *bpcore.MappingNode {
	if resolvedContext.partiallyResolved == nil {
		return nil
	}

	dataSource := resolvedContext.partiallyResolved.(*provider.ResolvedDataSource)
	if dataSource == nil {
		return nil
	}

	return dataSource.DataSourceMetadata.Custom
}

func getPartiallyResolvedIncludeVariables(
	resolvedContext *resolveContext,
) *bpcore.MappingNode {
	if resolvedContext.partiallyResolved == nil {
		return nil
	}

	include := resolvedContext.partiallyResolved.(*ResolvedInclude)
	if include == nil {
		return nil
	}

	return include.Variables
}

func getPartiallyResolvedIncludeMetadata(
	resolvedContext *resolveContext,
) *bpcore.MappingNode {
	if resolvedContext.partiallyResolved == nil {
		return nil
	}

	include := resolvedContext.partiallyResolved.(*ResolvedInclude)
	if include == nil {
		return nil
	}

	return include.Metadata
}

func getChildExport(
	exportName string,
	childState *state.InstanceState,
) *bpcore.MappingNode {
	if childState.Exports == nil {
		return nil
	}

	return childState.Exports[exportName]
}

func getValueFromStringMap(
	key string,
	stringMap *schema.StringMap,
) string {
	if stringMap == nil {
		return ""
	}

	return stringMap.Values[key]
}

func getValueFromMap(
	key string,
	mapNode *bpcore.MappingNode,
) *bpcore.MappingNode {
	if mapNode == nil {
		return nil
	}

	return mapNode.Fields[key]
}

func getItems(
	node *bpcore.MappingNode,
) []*bpcore.MappingNode {
	if node == nil || node.Items == nil {
		return nil
	}

	return node.Items
}

func getItem(
	items []*bpcore.MappingNode,
	index int,
) *bpcore.MappingNode {
	if len(items) <= index {
		return nil
	}

	return items[index]
}

func getFields(
	node *bpcore.MappingNode,
) map[string]*bpcore.MappingNode {
	if node == nil || node.Fields == nil {
		return nil
	}

	return node.Fields
}

func getField(
	fields map[string]*bpcore.MappingNode,
	fieldName string,
) *bpcore.MappingNode {
	if fields == nil {
		return nil
	}

	return fields[fieldName]
}

func getStringWithSubstitutions(
	node *bpcore.MappingNode,
) *substitutions.StringOrSubstitutions {
	if node != nil && node.StringWithSubstitutions != nil {
		return node.StringWithSubstitutions
	}

	return nil
}