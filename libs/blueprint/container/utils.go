package container

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/speccore"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/subengine"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
)

func deriveSpecFormat(specFilePath string) (schema.SpecFormat, error) {
	// Bear in mind this is a somewhat naive check, however if the spec file data
	// isn't valid YAML or JSON it will be caught in a failure to unmarshal
	// the spec.
	if strings.HasSuffix(specFilePath, ".yml") || strings.HasSuffix(specFilePath, ".yaml") {
		return schema.YAMLSpecFormat, nil
	}

	if strings.HasSuffix(specFilePath, ".json") {
		return schema.JSONSpecFormat, nil
	}

	return "", errUnsupportedSpecFileExtension(specFilePath)
}

// Provide a function compatible with loadSpec that simply returns an already defined format.
// This is useful for using the same functionality for loading from a string and from disk.
func predefinedFormatFactory(predefinedFormat schema.SpecFormat) func(input string) (schema.SpecFormat, error) {
	return func(input string) (schema.SpecFormat, error) {
		return predefinedFormat, nil
	}
}

func copyProviderMap(m map[string]provider.Provider) map[string]provider.Provider {
	copy := make(map[string]provider.Provider, len(m))
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

func collectLinksFromChain(
	ctx context.Context,
	chain *links.ChainLinkNode,
	refChainCollector validation.RefChainCollector,
) error {
	referencedByResourceID := core.ResourceElementID(chain.ResourceName)
	for _, link := range chain.LinksTo {
		linkImplementation, err := getLinkImplementation(chain, link)
		if err != nil {
			return err
		}

		linkKindOutput, err := linkImplementation.GetKind(ctx, &provider.LinkGetKindInput{})
		if err != nil {
			return err
		}

		if !alreadyCollected(refChainCollector, link, referencedByResourceID) {
			err = collectResourceFromLink(ctx, refChainCollector, link, linkKindOutput.Kind, referencedByResourceID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func collectResourceFromLink(
	ctx context.Context,
	refChainCollector validation.RefChainCollector,
	link *links.ChainLinkNode,
	linkKind provider.LinkKind,
	referencedByResourceID string,
) error {
	// Only collect link for cycle detection if it is a hard link.
	// Soft links do not require a specific order of deployment/resolution.
	if linkKind == provider.LinkKindHard {
		resourceID := core.ResourceElementID(link.ResourceName)
		err := refChainCollector.Collect(resourceID, link, referencedByResourceID, []string{"link"})
		if err != nil {
			return err
		}
	}

	// There is no risk of infinite recursion due to cyclic links as at this point,
	// any pure link cycles have been detected and reported.
	err := collectLinksFromChain(ctx, link, refChainCollector)
	if err != nil {
		return err
	}

	return nil
}

func alreadyCollected(
	refChainCollector validation.RefChainCollector,
	link *links.ChainLinkNode,
	referencedByResourceID string,
) bool {
	elementName := core.ResourceElementID(link.ResourceName)
	collected := refChainCollector.Chain(elementName)
	return collected != nil &&
		slices.ContainsFunc(
			collected.ReferencedBy,
			func(current *validation.ReferenceChainNode) bool {
				return current.ElementName == referencedByResourceID
			},
		)
}

func getChangesFromStageLinkChangesOutput(
	stageLinkChangesOutput *provider.LinkStageChangesOutput,
) provider.LinkChanges {
	if stageLinkChangesOutput == nil || stageLinkChangesOutput.Changes == nil {
		return provider.LinkChanges{}
	}

	return *stageLinkChangesOutput.Changes
}

func getLinkImplementation(
	linkA *links.ChainLinkNode,
	linkB *links.ChainLinkNode,
) (provider.Link, error) {
	linkImplementation, hasLinkImplementation := linkA.LinkImplementations[linkB.ResourceName]
	if !hasLinkImplementation {
		// The relationship could be either way.
		linkImplementation, hasLinkImplementation = linkB.LinkImplementations[linkA.ResourceName]
	}

	if !hasLinkImplementation {
		return nil, fmt.Errorf("no link implementation found between %s and %s", linkA.ResourceName, linkB.ResourceName)
	}

	return linkImplementation, nil
}

func extractChildRefNodes(
	blueprint *schema.Blueprint,
	refChainCollector validation.RefChainCollector,
) []*validation.ReferenceChainNode {
	childRefNodes := []*validation.ReferenceChainNode{}
	if blueprint.Include == nil {
		return childRefNodes
	}

	for childName := range blueprint.Include.Values {
		refChainNode := refChainCollector.Chain(core.ChildElementID(childName))
		if refChainNode != nil {
			childRefNodes = append(childRefNodes, refChainNode)
		}
	}

	return childRefNodes
}

func extractIncludeVariables(include *subengine.ResolvedInclude) map[string]*core.ScalarValue {
	includeVariables := map[string]*core.ScalarValue{}

	if include == nil || include.Variables == nil {
		return includeVariables
	}

	for variableName, variableValue := range include.Variables.Fields {
		if variableValue.Scalar != nil {
			includeVariables[variableName] = variableValue.Scalar
		}
	}

	return includeVariables
}

func isConditionKnownOnDeploy(
	resourceName string,
	resolveOnDeploy []string,
) bool {
	resourceElementID := core.ResourceElementID(resourceName)
	return slices.ContainsFunc(resolveOnDeploy, func(resolveOnDeployProp string) bool {
		conditionPropPrefix := fmt.Sprintf("%s.condition", resourceElementID)
		return strings.HasPrefix(resolveOnDeployProp, conditionPropPrefix)
	})
}

func evaluateCondition(
	condition *provider.ResolvedResourceCondition,
) bool {
	if condition == nil {
		return true
	}

	if condition.And != nil {
		result := true
		for _, subCondition := range condition.And {
			result = result && evaluateCondition(subCondition)
		}
		return result
	}

	if condition.Or != nil {
		result := false
		for _, subCondition := range condition.Or {
			result = result || evaluateCondition(subCondition)
		}
		return result
	}

	if condition.Not != nil {
		return !evaluateCondition(condition.Not)
	}

	if condition.StringValue != nil {
		return core.BoolValue(condition.StringValue)
	}

	// A condition with no value set is equivalent to a condition not being set at all
	// for the given resource.
	return true
}

func extractChildBlueprintFormat(includeName string, include *subengine.ResolvedInclude) (schema.SpecFormat, error) {
	if include == nil || include.Path == nil {
		return schema.SpecFormat(""), errMissingChildBlueprintPath(includeName)
	}

	pathString := core.StringValue(include.Path)
	if pathString == "" {
		// This should lead to an error when trying to load a child blueprint.
		return schema.SpecFormat(""), errEmptyChildBlueprintPath(includeName)
	}

	return deriveSpecFormat(pathString)
}

func flattenMapLists[Value any](m map[string][]Value) []Value {
	flattened := []Value{}
	for _, list := range m {
		flattened = append(flattened, list...)
	}
	return flattened
}

func invertMap[Value comparable](m map[string][]Value) map[Value]string {
	inverted := map[Value]string{}
	for key, values := range m {
		for _, value := range values {
			inverted[value] = key
		}
	}
	return inverted
}

func createLogicalLinkName(resourceAName string, resourceBName string) string {
	return fmt.Sprintf(
		"%s::%s",
		resourceAName,
		resourceBName,
	)
}

func getInstanceTreePath(
	params core.BlueprintParams,
	instanceID string,
) string {
	instanceTreePath := params.ContextVariable("instanceTreePath")
	if instanceTreePath == nil || instanceTreePath.StringValue == nil {
		return instanceID
	}

	parentTreePath := *instanceTreePath.StringValue
	return addToInstanceTreePath(parentTreePath, instanceID)
}

func addToInstanceTreePath(
	parentInstanceTreePath string,
	instanceID string,
) string {
	if parentInstanceTreePath == "" {
		return instanceID
	}

	return fmt.Sprintf("%s/%s", parentInstanceTreePath, instanceID)
}

func getIncludeTreePath(
	params core.BlueprintParams,
	includeChildIDName string,
) string {
	childName := strings.TrimPrefix(includeChildIDName, "children.")
	includeName := ""
	if childName != "" {
		includeName = fmt.Sprintf("include.%s", childName)
	}
	includeTreePath := params.ContextVariable("includeTreePath")
	if includeTreePath == nil || includeTreePath.StringValue == nil {
		return includeName
	}

	parentTreePath := *includeTreePath.StringValue
	return addToIncludeTreePath(parentTreePath, includeName)
}

func addToIncludeTreePath(
	parentIncludeTreePath string,
	includeName string,
) string {
	if parentIncludeTreePath == "" {
		return includeName
	}

	if includeName == "" {
		return parentIncludeTreePath
	}

	return fmt.Sprintf("%s::%s", parentIncludeTreePath, includeName)
}

func hasBlueprintCycle(
	parentInstanceTreePath string,
	instanceID string,
) bool {
	if parentInstanceTreePath == "" || instanceID == "" {
		return false
	}

	instances := strings.Split(parentInstanceTreePath, "/")
	return slices.Contains(instances, instanceID)
}

func createContextVarsForChildBlueprint(
	parentInstanceID string,
	instanceTreePath string,
	includeTreePath string,
) map[string]*core.ScalarValue {
	return map[string]*core.ScalarValue{
		"parentInstanceID": {
			StringValue: &parentInstanceID,
		},
		"instanceTreePath": {
			StringValue: &instanceTreePath,
		},
		"includeTreePath": {
			StringValue: &includeTreePath,
		},
	}
}

func createResourceTypeProviderMap(
	blueprintSpec speccore.BlueprintSpec,
	providers map[string]provider.Provider,
) map[string]provider.Provider {
	resourceTypeProviderMap := map[string]provider.Provider{}
	resources := map[string]*schema.Resource{}
	if blueprintSpec.Schema().Resources != nil {
		resources = blueprintSpec.Schema().Resources.Values
	}

	for _, resource := range resources {
		namespace := strings.Split(resource.Type.Value, "/")[0]
		resourceTypeProviderMap[resource.Type.Value] = providers[namespace]
	}
	return resourceTypeProviderMap
}

func createResourceProviderMap(
	blueprintSpec speccore.BlueprintSpec,
	providers map[string]provider.Provider,
) map[string]provider.Provider {
	resourceProviderMap := map[string]provider.Provider{}
	resources := map[string]*schema.Resource{}
	if blueprintSpec.Schema().Resources != nil {
		resources = blueprintSpec.Schema().Resources.Values
	}

	for resourceName, resource := range resources {
		namespace := strings.Split(resource.Type.Value, "/")[0]
		resourceProviderMap[resourceName] = providers[namespace]
	}
	return resourceProviderMap
}

// Finds direct dependents of a given element in a blueprint instance.
// This should not be used for finding transitive dependents.
func findDependents(
	dependeeElement state.Element,
	nodesToBeDeployed []*DeploymentNode,
	instanceState *state.InstanceState,
) *CollectedElements {
	dependents := &CollectedElements{
		Resources: []*ResourceIDInfo{},
		Children:  []*ChildBlueprintIDInfo{},
		Total:     0,
	}

	for _, node := range nodesToBeDeployed {
		if node.Type() == "resource" {
			collectDependentResource(node, dependeeElement, instanceState, dependents)
		} else if node.Type() == "child" {
			collectDependentChildBlueprint(node, dependeeElement, instanceState, dependents)
		}
	}

	return dependents
}

func collectDependentResource(
	potentialDependentNode *DeploymentNode,
	dependeeStateElement state.Element,
	instanceState *state.InstanceState,
	dependents *CollectedElements,
) {
	currentResourceName := potentialDependentNode.ChainLinkNode.ResourceName
	currentNodeResourceState := getResourceStateByName(instanceState, currentResourceName)
	if currentNodeResourceState != nil {
		elementTypeDependencies := getResourceElementTypeDependencies(
			dependeeStateElement.Kind(),
			currentNodeResourceState,
		)
		if slices.Contains(
			elementTypeDependencies,
			dependeeStateElement.ID(),
		) {
			dependents.Resources = append(dependents.Resources, &ResourceIDInfo{
				ResourceID:   currentNodeResourceState.ResourceID,
				ResourceName: currentResourceName,
			})
			dependents.Total += 1
		}
	}
}

func getResourceElementTypeDependencies(
	dependeeType state.ElementKind,
	dependentResourceState *state.ResourceState,
) []string {
	dependencies := []string{}

	if dependeeType == state.ResourceElement {
		dependencies = dependentResourceState.DependsOnResources
	}

	if dependeeType == state.ChildElement {
		dependencies = dependentResourceState.DependsOnChildren
	}

	return dependencies
}

func collectDependentChildBlueprint(
	potentialDependentNode *DeploymentNode,
	dependeeStateElement state.Element,
	instanceState *state.InstanceState,
	dependents *CollectedElements,
) {
	currentChildName := strings.TrimPrefix(potentialDependentNode.ChildNode.ElementName, "children.")
	currentNodeChildState := getChildStateByName(instanceState, currentChildName)
	childDependencies := getChildDependencies(instanceState, currentChildName)
	if currentNodeChildState != nil {
		elementTypeDependencies := getChildElementTypeDependencies(
			dependeeStateElement.Kind(),
			childDependencies,
		)
		if slices.Contains(
			elementTypeDependencies,
			dependeeStateElement.ID(),
		) {
			dependents.Children = append(dependents.Children, &ChildBlueprintIDInfo{
				ChildInstanceID: currentNodeChildState.InstanceID,
				ChildName:       currentChildName,
			})
			dependents.Total += 1
		}
	}
}

func getChildElementTypeDependencies(
	dependeeType state.ElementKind,
	childDependencies *state.ChildDependencyInfo,
) []string {
	dependencies := []string{}

	if childDependencies == nil {
		return dependencies
	}

	if dependeeType == state.ResourceElement {
		dependencies = childDependencies.DependsOnResources
	}

	if dependeeType == state.ChildElement {
		dependencies = childDependencies.DependsOnChildren
	}

	return dependencies
}

func collectedElementsHasResource(
	searchIn *CollectedElements,
	resourceInfo *ResourceIDInfo,
) bool {
	return slices.ContainsFunc(
		searchIn.Resources,
		func(compareWith *ResourceIDInfo) bool {
			return compareWith.ResourceName == resourceInfo.ResourceName
		},
	)
}

func collectedElementsHasChild(
	searchIn *CollectedElements,
	childInfo *ChildBlueprintIDInfo,
) bool {
	return slices.ContainsFunc(
		searchIn.Children,
		func(compareWith *ChildBlueprintIDInfo) bool {
			return compareWith.ChildName == childInfo.ChildName
		},
	)
}

func filterOutRecreated(
	searchIn *CollectedElements,
	changes *BlueprintChanges,
) *CollectedElements {
	filtered := &CollectedElements{
		Resources: []*ResourceIDInfo{},
		Children:  []*ChildBlueprintIDInfo{},
		Total:     0,
	}

	for _, resourceInfo := range searchIn.Resources {
		plannedChanges, hasPlannedChanges := changes.ResourceChanges[resourceInfo.ResourceName]
		if hasPlannedChanges && !plannedChanges.MustRecreate {
			filtered.Resources = append(filtered.Resources, resourceInfo)
			filtered.Total += 1
		}
	}

	for _, childInfo := range searchIn.Children {
		isRecreatePlanned := slices.Contains(changes.RecreateChildren, childInfo.ChildName)
		if !isRecreatePlanned {
			filtered.Children = append(filtered.Children, childInfo)
			filtered.Total += 1
		}
	}

	return filtered
}

func getProviderResourceImplementation(
	ctx context.Context,
	resourceName string,
	resourceType string,
	resourceProviders map[string]provider.Provider,
) (provider.Resource, error) {
	resourceProvider, hasResourceProvider := resourceProviders[resourceName]
	if !hasResourceProvider {
		return nil, fmt.Errorf("no provider found for resource %q", resourceName)
	}

	return resourceProvider.Resource(ctx, resourceType)
}

func getResourceStateByName(
	instanceState *state.InstanceState,
	resourceName string,
) *state.ResourceState {
	resourceID, hasResourceID := instanceState.ResourceIDs[resourceName]
	if !hasResourceID {
		return nil
	}

	resourceState, hasResourceState := instanceState.Resources[resourceID]
	if !hasResourceState {
		return nil
	}

	return resourceState
}

func getChildStateByName(
	instanceState *state.InstanceState,
	childName string,
) *state.InstanceState {
	childBlueprint, hasChildBlueprint := instanceState.ChildBlueprints[childName]
	if !hasChildBlueprint {
		return nil
	}

	return childBlueprint
}

func getLinkStateByName(
	instanceState *state.InstanceState,
	linkName string,
) *state.LinkState {
	linkState, hasLinkState := instanceState.Links[linkName]
	if !hasLinkState {
		return nil
	}

	return linkState
}

func getChildDependencies(
	instanceState *state.InstanceState,
	childName string,
) *state.ChildDependencyInfo {
	childDeps, hasChildDeps := instanceState.ChildDependencies[childName]
	if !hasChildDeps {
		return nil
	}

	return childDeps
}

func getResourceInfoFromStateForLinkRemoval(
	instanceState *state.InstanceState,
	resourceName string,
) *provider.ResourceInfo {
	resourceState := getResourceStateByName(instanceState, resourceName)
	if resourceState == nil {
		return nil
	}

	return &provider.ResourceInfo{
		ResourceID:           resourceState.ResourceID,
		ResourceName:         resourceName,
		InstanceID:           instanceState.InstanceID,
		CurrentResourceState: resourceState,
	}
}

func getPartiallyResolvedResourceFromChanges(
	changes *BlueprintChanges,
	resourceName string,
) *provider.ResolvedResource {
	if changes == nil {
		return nil
	}

	resourceChanges, hasResourceChanges := changes.ResourceChanges[resourceName]
	if !hasResourceChanges {
		resourceChanges, hasResourceChanges = changes.NewResources[resourceName]
		if !hasResourceChanges {
			return nil
		}
	}

	return resourceChanges.AppliedResourceInfo.ResourceWithResolvedSubs
}

func extractLinkDirectDependencies(logicalLinkName string) *linkDependencyInfo {
	parts := strings.Split(logicalLinkName, "::")
	if len(parts) != 2 {
		return nil
	}

	return &linkDependencyInfo{
		resourceAName: parts[0],
		resourceBName: parts[1],
	}
}

func getResourceTypesForLink(linkName string, currentState *state.InstanceState) (string, string, error) {
	linkDependencyInfo := extractLinkDirectDependencies(linkName)
	if linkDependencyInfo == nil {
		return "", "", errInvalidLogicalLinkName(
			linkName,
			currentState.InstanceID,
		)
	}

	resourceAState := getResourceStateByName(currentState, linkDependencyInfo.resourceAName)
	if resourceAState == nil {
		return "", "", errResourceNotFoundInState(
			currentState.InstanceID,
			linkDependencyInfo.resourceAName,
		)
	}

	resourceBState := getResourceStateByName(currentState, linkDependencyInfo.resourceBName)
	if resourceBState == nil {
		return "", "", errResourceNotFoundInState(
			currentState.InstanceID,
			linkDependencyInfo.resourceBName,
		)
	}

	return resourceAState.ResourceType, resourceBState.ResourceType, nil
}

func getResourceInfo(
	ctx context.Context,
	stageInfo *stageResourceChangeInfo,
	substitutionResolver subengine.SubstitutionResolver,
	resourceCache *core.Cache[*provider.ResolvedResource],
	stateContainer state.Container,
) (*provider.ResourceInfo, *subengine.ResolveInResourceResult, error) {
	resolveResourceResult, err := substitutionResolver.ResolveInResource(
		ctx,
		stageInfo.node.ResourceName,
		stageInfo.node.Resource,
		&subengine.ResolveResourceTargetInfo{
			ResolveFor: subengine.ResolveForChangeStaging,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	_, cached := resourceCache.Get(stageInfo.node.ResourceName)
	if !cached {
		resourceCache.Set(
			stageInfo.node.ResourceName,
			resolveResourceResult.ResolvedResource,
		)
	}

	var currentResourceStatePtr *state.ResourceState
	resources := stateContainer.Resources()
	currentResourceState, err := resources.GetByName(
		ctx,
		stageInfo.instanceID,
		stageInfo.node.ResourceName,
	)
	if err != nil {
		if !state.IsResourceNotFound(err) {
			return nil, nil, err
		}
	} else {
		currentResourceStatePtr = &currentResourceState
	}

	return &provider.ResourceInfo{
		ResourceID:               stageInfo.resourceID,
		ResourceName:             stageInfo.node.ResourceName,
		InstanceID:               stageInfo.instanceID,
		CurrentResourceState:     currentResourceStatePtr,
		ResourceWithResolvedSubs: resolveResourceResult.ResolvedResource,
	}, resolveResourceResult, nil
}

func toFullLinkPath(
	resourceAName string,
	resourceBName string,
) func(string, int) string {
	return func(linkFieldPath string, _ int) string {
		linkName := linkElementID(
			createLogicalLinkName(resourceAName, resourceBName),
		)
		if strings.HasPrefix(linkFieldPath, "[") {
			return fmt.Sprintf("%s%s", linkName, linkFieldPath)
		}
		return fmt.Sprintf("%s.%s", linkName, linkFieldPath)
	}
}

func toFullResourcePath(
	resourceName string,
) func(string, int) string {
	return func(resourceFieldPath string, _ int) string {
		resourceName := core.ResourceElementID(resourceName)
		if strings.HasPrefix(resourceFieldPath, "[") {
			return fmt.Sprintf("%s%s", resourceName, resourceFieldPath)
		}
		return fmt.Sprintf("%s.%s", resourceName, resourceFieldPath)
	}
}

func getNamespacedLogicalName(element state.Element) string {
	switch element.Kind() {
	case state.ResourceElement:
		return core.ResourceElementID(element.LogicalName())
	case state.ChildElement:
		return core.ChildElementID(element.LogicalName())
	case state.LinkElement:
		return linkElementID(element.LogicalName())
	default:
		return ""
	}
}

func copyPointerMap[Item any](input map[string]*Item) map[string]Item {
	output := map[string]Item{}
	for key, value := range input {
		output[key] = *value
	}
	return output
}

func exceedsMaxDepth(path string, maxDepth int) bool {
	return len(strings.Split(path, "/")) > maxDepth
}

func anyEmptyString(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}
