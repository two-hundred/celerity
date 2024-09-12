package languageserver

import (
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/substitutions"
	common "github.com/two-hundred/ls-builder/common"
	lsp "github.com/two-hundred/ls-builder/lsp_3_17"
	"go.uber.org/zap"
)

// HoverContent represents the content for a hover message.
type HoverContent struct {
	Value string
	Range *lsp.Range
}

// GetHoverContent returns the hover content for the given position in the
// source document.
func GetHoverContent(
	ctx *common.LSPContext,
	tree *schema.TreeNode,
	blueprint *schema.Blueprint,
	params *lsp.TextDocumentPositionParams,
	funcRegistry provider.FunctionRegistry,
	resourceRegistry resourcehelpers.Registry,
	dataSourceRegistry provider.DataSourceRegistry,
	logger *zap.Logger,
) (*HoverContent, error) {

	// The last element in the collected list is the element with the shortest
	// range that contains the position.
	collected := []*schema.TreeNode{}
	collectElementsAtPosition(tree, params.Position, logger, &collected)

	return getHoverElementContent(
		ctx,
		blueprint,
		collected,
		funcRegistry,
		resourceRegistry,
		dataSourceRegistry,
		logger,
	)
}

func getHoverElementContent(
	ctx *common.LSPContext,
	blueprint *schema.Blueprint,
	collected []*schema.TreeNode,
	funcRegistry provider.FunctionRegistry,
	resourceRegistry resourcehelpers.Registry,
	dataSourceRegistry provider.DataSourceRegistry,
	logger *zap.Logger,
) (*HoverContent, error) {

	if len(collected) == 0 {
		return &HoverContent{}, nil
	}

	// Work backwards through the collected elements to find the first element
	// of a type that supports hover content.
	var node *schema.TreeNode
	var elementType string
	i := len(collected) - 1
	for node == nil && i >= 0 {
		pathParts := strings.Split(collected[i].Path, "/")
		node, elementType = matchHoverElement(collected, i, pathParts)
		i -= 1
	}

	switch elementType {
	case "functionCall":
		return getFunctionCallHoverContent(ctx, node, funcRegistry, logger)
	case "varRef":
		return getVarRefHoverContent(node, blueprint)
	case "valRef":
		return getValRefHoverContent(node, blueprint)
	case "childRef":
		return getChildRefHoverContent(node, blueprint)
	case "resourceRef":
		return getResourceRefHoverContent(ctx, node, blueprint, resourceRegistry, logger)
	case "datasourceRef":
		return getDataSourceRefHoverContent(node, blueprint)
	case "elemRef":
		return getElemRefHoverContent(node, blueprint)
	case "elemIndexRef":
		return getElemIndexRefHoverContent(node, blueprint)
	case "resourceType":
		return getResourceTypeHoverContent(ctx, node, resourceRegistry, logger)
	case "dataSourceType":
		return getDataSourceTypeHoverContent(ctx, node, dataSourceRegistry, logger)
	default:
		return &HoverContent{}, nil
	}
}

func matchHoverElement(
	collected []*schema.TreeNode,
	index int,
	pathParts []string,
) (*schema.TreeNode, string) {

	if isFunctionCallPath(pathParts) {
		return collected[index], "functionCall"
	} else if isVarRefPath(pathParts) {
		return collected[index], "varRef"
	} else if isValRefPath(pathParts) {
		return collected[index], "valRef"
	} else if isChildRefPath(pathParts) {
		return collected[index], "childRef"
	} else if isResourceRefPath(pathParts) {
		return collected[index], "resourceRef"
	} else if isDataSourceRefPath(pathParts) {
		return collected[index], "datasourceRef"
	} else if isElemRefPath(pathParts) {
		return collected[index], "elemRef"
	} else if isElemIndexRefPath(pathParts) {
		return collected[index], "elemIndexRef"
	} else if isResourceTypePath(pathParts) {
		return collected[index], "resourceType"
	} else if isDataSourceTypePath(pathParts) {
		return collected[index], "dataSourceType"
	}

	return nil, ""
}

func isFunctionCallPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "functionCall"
}

func isVarRefPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "varRef"
}

func isValRefPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "valRef"
}

func isChildRefPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "childRef"
}

func isResourceRefPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "resourceRef"
}

func isDataSourceRefPath(pathParts []string) bool {
	return len(pathParts) > 1 && pathParts[len(pathParts)-2] == "datasourceRef"
}

func isElemRefPath(pathParts []string) bool {
	return len(pathParts) >= 1 && pathParts[len(pathParts)-1] == "elemRef"
}

func isElemIndexRefPath(pathParts []string) bool {
	return len(pathParts) >= 1 && pathParts[len(pathParts)-1] == "elemIndexRef"
}

func isResourceTypePath(pathParts []string) bool {
	return len(pathParts) > 2 &&
		pathParts[len(pathParts)-3] == "resources" &&
		pathParts[len(pathParts)-1] == "type"
}

func isDataSourceTypePath(pathParts []string) bool {
	return len(pathParts) > 2 &&
		pathParts[len(pathParts)-3] == "datasources" &&
		pathParts[len(pathParts)-1] == "type"
}

func getFunctionCallHoverContent(
	ctx *common.LSPContext,
	node *schema.TreeNode,
	funcRegistry provider.FunctionRegistry,
	logger *zap.Logger,
) (*HoverContent, error) {

	subFunc, isSubFunc := node.SchemaElement.(*substitutions.SubstitutionFunctionExpr)
	if !isSubFunc {
		return &HoverContent{}, nil
	}

	signatureInfo, err := signatureInfoFromFunction(subFunc, ctx, funcRegistry, logger)
	if err != nil {
		return &HoverContent{}, err
	}

	content := customRenderSignatures(signatureInfo)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getVarRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	varRef, isVarRef := node.SchemaElement.(*substitutions.SubstitutionVariable)
	if !isVarRef {
		return &HoverContent{}, nil
	}

	variable := getVariable(blueprint, varRef.VariableName)
	if variable == nil {
		return &HoverContent{}, nil
	}

	content := renderVariableInfo(node.Label, variable)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getValRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	valRef, isValRef := node.SchemaElement.(*substitutions.SubstitutionValueReference)
	if !isValRef {
		return &HoverContent{}, nil
	}

	value := getValue(blueprint, valRef.ValueName)
	if value == nil {
		return &HoverContent{}, nil
	}

	content := renderValueInfo(node.Label, value)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getChildRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	childRef, isChildRef := node.SchemaElement.(*substitutions.SubstitutionChild)
	if !isChildRef {
		return &HoverContent{}, nil
	}

	child := getChild(blueprint, childRef.ChildName)
	if child == nil {
		return &HoverContent{}, nil
	}

	content := renderChildInfo(node.Label, child)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getResourceRefHoverContent(
	ctx *common.LSPContext,
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
	resourceRegistry resourcehelpers.Registry,
	logger *zap.Logger,
) (*HoverContent, error) {
	resRef, isResRef := node.SchemaElement.(*substitutions.SubstitutionResourceProperty)
	if !isResRef {
		return &HoverContent{}, nil
	}

	resource := getResource(blueprint, resRef.ResourceName)
	if resource == nil || resource.Type == nil {
		return &HoverContent{}, nil
	}

	if len(resRef.Path) == 0 {
		return getBasicResourceHoverContent(node.Label, resource), nil
	}

	if len(resRef.Path) > 1 && resRef.Path[0].FieldName == "spec" {
		logger.Debug(
			"Fetching spec definition for hover content",
			zap.String("resourceType", resource.Type.Value),
		)
		specDefOutput, err := resourceRegistry.GetSpecDefinition(
			ctx.Context,
			resource.Type.Value,
			&provider.ResourceGetSpecDefinitionInput{},
		)
		if err != nil {
			return &HoverContent{}, err
		}

		return getResourceWithSpecHoverContent(
			node,
			resource,
			resRef,
			specDefOutput.SpecDefinition,
		)
	}

	if len(resRef.Path) > 1 && resRef.Path[0].FieldName == "state" {
		logger.Debug(
			"Fetching state definition for hover content",
			zap.String("resourceType", resource.Type.Value),
		)
		stateDefOutput, err := resourceRegistry.GetStateDefinition(
			ctx.Context,
			resource.Type.Value,
			&provider.ResourceGetStateDefinitionInput{},
		)
		if err != nil {
			return &HoverContent{}, err
		}

		return getResourceWithStateHoverContent(
			node,
			resource,
			resRef,
			stateDefOutput.StateDefinition,
		)
	}

	return &HoverContent{}, nil
}

func getBasicResourceHoverContent(
	resourceName string,
	resource *schema.Resource,
) *HoverContent {
	content := renderBasicResourceInfo(resourceName, resource)

	return &HoverContent{
		Value: content,
		Range: nil,
	}
}

func getResourceWithSpecHoverContent(
	node *schema.TreeNode,
	resource *schema.Resource,
	resRef *substitutions.SubstitutionResourceProperty,
	specDef *provider.ResourceSpecDefinition,
) (*HoverContent, error) {
	if specDef == nil {
		return &HoverContent{}, nil
	}

	specFieldSchema, err := findResourceFieldSchema(specDef.Schema, resRef.Path[1:])
	if err != nil {
		return &HoverContent{}, err
	}

	content := renderResourceDefinitionFieldInfo(
		node.Label,
		resource,
		resRef,
		specFieldSchema,
	)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getResourceWithStateHoverContent(
	node *schema.TreeNode,
	resource *schema.Resource,
	resRef *substitutions.SubstitutionResourceProperty,
	stateDef *provider.ResourceStateDefinition,
) (*HoverContent, error) {
	if stateDef == nil {
		return &HoverContent{}, nil
	}

	stateFieldSchema, err := findResourceFieldSchema(stateDef.Schema, resRef.Path[1:])
	if err != nil {
		return &HoverContent{}, err
	}

	content := renderResourceDefinitionFieldInfo(
		node.Label,
		resource,
		resRef,
		stateFieldSchema,
	)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getDataSourceRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	dataSourceRef, isDataSourceRef := node.SchemaElement.(*substitutions.SubstitutionDataSourceProperty)
	if !isDataSourceRef {
		return &HoverContent{}, nil
	}

	dataSource := getDataSource(blueprint, dataSourceRef.DataSourceName)
	if dataSource == nil {
		return &HoverContent{}, nil
	}

	dataSourceField := getDataSourceField(dataSource, dataSourceRef.FieldName)
	if dataSourceField == nil {
		return &HoverContent{
			Value: renderBasicDataSourceInfo(node.Label, dataSource),
			Range: rangeToLSPRange(node.Range),
		}, nil
	}

	content := renderDataSourceFieldInfo(node.Label, dataSource, dataSourceRef, dataSourceField)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getElemRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	elemRef, isElemRef := node.SchemaElement.(*substitutions.SubstitutionElemReference)
	if !isElemRef {
		return &HoverContent{}, nil
	}

	resourceName := extractResourceNameFromElemRef(node.Path)
	resource := getResource(blueprint, resourceName)
	if resource == nil {
		return &HoverContent{}, nil
	}

	content := renderElemRefInfo(resourceName, resource, elemRef)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getElemIndexRefHoverContent(
	node *schema.TreeNode,
	blueprint *schema.Blueprint,
) (*HoverContent, error) {

	resourceName := extractResourceNameFromElemRef(node.Path)
	resource := getResource(blueprint, resourceName)
	if resource == nil {
		return &HoverContent{}, nil
	}

	content := renderElemIndexRefInfo(resourceName, resource)

	return &HoverContent{
		Value: content,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getResourceTypeHoverContent(
	ctx *common.LSPContext,
	node *schema.TreeNode,
	resourceRegistry resourcehelpers.Registry,
	logger *zap.Logger,
) (*HoverContent, error) {
	resType, isResType := node.SchemaElement.(*schema.ResourceTypeWrapper)
	if !isResType || resType == nil {
		return &HoverContent{}, nil
	}

	logger.Debug(
		"Fetching resource type definition for hover content",
		zap.String("resourceType", resType.Value),
	)
	descriptionOutput, err := resourceRegistry.GetTypeDescription(
		ctx.Context,
		resType.Value,
		&provider.ResourceGetTypeDescriptionInput{},
	)
	if err != nil {
		return &HoverContent{}, err
	}

	description := descriptionOutput.MarkdownDescription
	if description == "" {
		description = descriptionOutput.PlainTextDescription
	}

	return &HoverContent{
		Value: description,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func getDataSourceTypeHoverContent(
	ctx *common.LSPContext,
	node *schema.TreeNode,
	dataSourceRegistry provider.DataSourceRegistry,
	logger *zap.Logger,
) (*HoverContent, error) {
	dataSourceType, isDataSourceType := node.SchemaElement.(*schema.DataSourceTypeWrapper)
	if !isDataSourceType || dataSourceType == nil {
		return &HoverContent{}, nil
	}

	logger.Debug(
		"Fetching data source type definition for hover content",
		zap.String("dataSourceType", dataSourceType.Value),
	)
	descriptionOutput, err := dataSourceRegistry.GetTypeDescription(
		ctx.Context,
		dataSourceType.Value,
		&provider.DataSourceGetTypeDescriptionInput{},
	)
	if err != nil {
		return &HoverContent{}, err
	}

	description := descriptionOutput.MarkdownDescription
	if description == "" {
		description = descriptionOutput.PlainTextDescription
	}

	return &HoverContent{
		Value: description,
		Range: rangeToLSPRange(node.Range),
	}, nil
}

func collectElementsAtPosition(
	tree *schema.TreeNode,
	pos lsp.Position,
	logger *zap.Logger,
	collected *[]*schema.TreeNode,
) {
	if tree == nil {
		return
	}

	if containsLSPPoint(tree.Range, pos) {
		*collected = append(*collected, tree)
		i := 0
		for i < len(tree.Children) {
			collectElementsAtPosition(tree.Children[i], pos, logger, collected)
			i += 1
		}
	}
}

func getVariable(blueprint *schema.Blueprint, name string) *schema.Variable {
	if blueprint.Variables == nil || blueprint.Variables.Values == nil {
		return nil
	}

	variable, hasVariable := blueprint.Variables.Values[name]
	if !hasVariable {
		return nil
	}

	return variable
}

func getValue(blueprint *schema.Blueprint, name string) *schema.Value {
	if blueprint.Values == nil || blueprint.Values.Values == nil {
		return nil
	}

	value, hasValue := blueprint.Values.Values[name]
	if !hasValue {
		return nil
	}

	return value
}

func getChild(blueprint *schema.Blueprint, name string) *schema.Include {
	if blueprint.Include == nil || blueprint.Include.Values == nil {
		return nil
	}

	child, hasChild := blueprint.Include.Values[name]
	if !hasChild {
		return nil
	}

	return child
}

func getResource(blueprint *schema.Blueprint, name string) *schema.Resource {
	if blueprint.Resources == nil || blueprint.Resources.Values == nil {
		return nil
	}

	resource, hasResource := blueprint.Resources.Values[name]
	if !hasResource {
		return nil
	}

	return resource
}

func getDataSource(blueprint *schema.Blueprint, name string) *schema.DataSource {
	if blueprint.DataSources == nil || blueprint.DataSources.Values == nil {
		return nil
	}

	dataSource, hasDataSource := blueprint.DataSources.Values[name]
	if !hasDataSource {
		return nil
	}

	return dataSource
}

func getDataSourceField(dataSource *schema.DataSource, name string) *schema.DataSourceFieldExport {
	if dataSource.Exports == nil || dataSource.Exports.Values == nil {
		return nil
	}

	field, hasField := dataSource.Exports.Values[name]
	if !hasField {
		return nil
	}

	return field
}

func findResourceFieldSchema(
	defSchema *provider.ResourceDefinitionsSchema,
	path []*substitutions.SubstitutionPathItem,
) (*provider.ResourceDefinitionsSchema, error) {
	if len(path) == 0 {
		return nil, nil
	}

	currentSchema := defSchema
	i := 0
	for currentSchema != nil && i < len(path) {
		pathItem := path[i]

		objectFieldSchema := checkResourceObjectFieldSchema(currentSchema, pathItem)
		if objectFieldSchema != nil {
			currentSchema = objectFieldSchema
		}

		mapFieldSchema := checkResourceMapFieldSchema(currentSchema, pathItem)
		if mapFieldSchema != nil {
			currentSchema = mapFieldSchema
		}

		arrayItemSchema := checkResourceArrayItemSchema(currentSchema, pathItem)
		if arrayItemSchema != nil {
			currentSchema = arrayItemSchema
		}

		if objectFieldSchema == nil && mapFieldSchema == nil && arrayItemSchema == nil {
			// Avoid associating the field with parent schemas,
			// this will create confusing docs/help information that suggests
			// a given field has a type that it does not.
			currentSchema = nil
		}

		i += 1
	}

	return currentSchema, nil
}

func checkResourceObjectFieldSchema(
	schema *provider.ResourceDefinitionsSchema,
	pathItem *substitutions.SubstitutionPathItem,
) *provider.ResourceDefinitionsSchema {

	if pathItem.FieldName != "" &&
		schema.Type == provider.ResourceDefinitionsSchemaTypeObject {
		fieldSchema, hasField := schema.Attributes[pathItem.FieldName]
		if !hasField {
			return nil
		} else {
			return fieldSchema
		}
	}

	return nil
}

func checkResourceMapFieldSchema(
	schema *provider.ResourceDefinitionsSchema,
	pathItem *substitutions.SubstitutionPathItem,
) *provider.ResourceDefinitionsSchema {

	if pathItem.FieldName != "" &&
		schema.Type == provider.ResourceDefinitionsSchemaTypeMap {
		if schema.MapValues == nil {
			return nil
		} else {
			return schema.MapValues
		}
	}

	return nil
}

func checkResourceArrayItemSchema(
	schema *provider.ResourceDefinitionsSchema,
	pathItem *substitutions.SubstitutionPathItem,
) *provider.ResourceDefinitionsSchema {

	if pathItem.PrimitiveArrIndex != nil &&
		schema.Type == provider.ResourceDefinitionsSchemaTypeArray {
		if schema.Items == nil {
			return nil
		} else {
			return schema.Items
		}
	}

	return nil
}

func extractResourceNameFromElemRef(
	elemRefPath string,
) string {
	pathParts := strings.Split(elemRefPath, "/")
	if len(pathParts) < 4 || (len(pathParts) > 1 && pathParts[1] != "resources") {
		// "/resources/<resourceName>/.*?(elemRef | elemIndexRef)"
		// must contain at least 4 parts to be a valid elemRef
		// path. "" "resources" "<resourceName>" ... ( "elemRef" | "elemIndexRef" )
		return ""
	}

	return pathParts[2]
}
