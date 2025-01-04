package container

import (
	"slices"
	"sync"

	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	commoncore "github.com/two-hundred/celerity/libs/common/core"
)

// ChangeStagingState provides functionality for tracking and setting the state when staging
// changes for a deployment.
// In most cases, this is to be treated as ephemeral state that lasts for the duration
// of a change staging operation.
type ChangeStagingState interface {
	// AddElementsThatMustBeRecreated adds elements that must be
	// recreated due to removal of dependencies.
	// This adds the elements to the existing set of elements that must be recreated
	// and does not replace the existing set.
	AddElementsThatMustBeRecreated(mustRecreate *CollectedElements)
	// ApplyResourceChanges applies the changes for a given resource to the staging state.
	ApplyResourceChanges(changes ResourceChangesMessage)
	// UpdateLinkStagingState updates the staging state for links containing
	// the resource represented by the provided ChainLinkNode.
	UpdateLinkStagingState(node *links.ChainLinkNode) []*LinkPendingCompletion
	// MustRecreateResourceOnRemovedDependencies returns true if the resource
	// represented by the provided resource name must be recreated
	// due to the removal of dependencies.
	MustRecreateResourceOnRemovedDependencies(resourceName string) bool
	// CountPendingLinksForGroup returns the number of pending links for the
	// provided group of nodes for the current change staging operation.
	CountPendingLinksForGroup(group []*DeploymentNode) int
	// ApplyLinkChanges applies the changes for a given link to the staging state.
	ApplyLinkChanges(changes LinkChangesMessage)
	// ApplyChildChanges applies the changes for a given child blueprint
	// to the staging state.
	ApplyChildChanges(changes ChildChangesMessage)
	// GetResourceChanges returns the changes for the provided resource name
	// from the staging state.
	// If no changes are found for the provided resource name, nil is returned.
	GetResourceChanges(resourceName string) *provider.Changes
	// MarkLinkAsNoLongerPending marks the link between the provided resource nodes
	// as no longer pending in the staging state.
	MarkLinkAsNoLongerPending(resourceANode, resourceBNode *links.ChainLinkNode)
	// UpdateExportChanges updates the export changes in the staging state.
	UpdateExportChanges(collectedExportChanges *intermediaryBlueprintChanges)
	// ExtractBlueprintChanges extracts the changes that have been staged
	// for the deployment to be sent to the client initiating the change staging operation.
	ExtractBlueprintChanges() BlueprintChanges
}

// NewDefaultChangeStagingState creates a new instance of the default
// implementation for tracking and setting the state of staging changes
// for a deployment.
// The default implementation is a thread-safe, ephemeral store for stage changing state.
func NewDefaultChangeStagingState() ChangeStagingState {
	return &defaultChangeStagingState{
		pendingLinks:        make(map[string]*LinkPendingCompletion),
		resourceNameLinkMap: make(map[string][]string),
		outputChanges:       &intermediaryBlueprintChanges{},
		mustRecreate: &CollectedElements{
			Resources: []*ResourceIDInfo{},
			Children:  []*ChildBlueprintIDInfo{},
			Total:     0,
		},
	}
}

type defaultChangeStagingState struct {
	// A mapping of a link name to the pending link completion state.
	// A link ID in this context is made up of the resource names of the two resources
	// that are linked together.
	// For example, if resource A is linked to resource B, the link name would be "A::B".
	pendingLinks map[string]*LinkPendingCompletion
	// A mapping of resource names to pending links that include the resource.
	resourceNameLinkMap map[string][]string
	// The full set of changes that will be sent to the caller-provided complete channel
	// when all changes have been staged.
	// This is an intermediary format that holds pointers to resource change sets to allow
	// modification without needing to copy and patch resource change sets back in to the state
	// each time resource change set state needs to be updated with link change sets.
	outputChanges *intermediaryBlueprintChanges
	// A set of elements that must be recreated due to removal of dependencies.
	mustRecreate *CollectedElements
	// Mutex is required as resources can be staged concurrently.
	mu sync.Mutex
}

type intermediaryBlueprintChanges struct {
	NewResources     map[string]*provider.Changes
	ResourceChanges  map[string]*provider.Changes
	RemovedResources []string
	RemovedLinks     []string
	NewChildren      map[string]*NewBlueprintDefinition
	ChildChanges     map[string]*BlueprintChanges
	RemovedChildren  []string
	NewExports       map[string]*provider.FieldChange
	ExportChanges    map[string]*provider.FieldChange
	RemovedExports   []string
	UnchangedExports []string
	ResolveOnDeploy  []string
}

func (c *defaultChangeStagingState) AddElementsThatMustBeRecreated(mustRecreate *CollectedElements) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, resource := range mustRecreate.Resources {
		if !collectedElementsHasResource(c.mustRecreate, resource) {
			c.mustRecreate.Resources = append(c.mustRecreate.Resources, resource)
			c.mustRecreate.Total += 1
		}
	}

	for _, child := range mustRecreate.Children {
		if !collectedElementsHasChild(c.mustRecreate, child) {
			c.mustRecreate.Children = append(c.mustRecreate.Children, child)
			c.mustRecreate.Total += 1
		}
	}
}

func (c *defaultChangeStagingState) ApplyResourceChanges(changes ResourceChangesMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if changes.New {
		if c.outputChanges.NewResources == nil {
			c.outputChanges.NewResources = map[string]*provider.Changes{}
		}
		c.outputChanges.NewResources[changes.ResourceName] = &changes.Changes
	} else if changes.Removed {
		if c.outputChanges.RemovedResources == nil {
			c.outputChanges.RemovedResources = []string{}
		}
		c.outputChanges.RemovedResources = append(
			c.outputChanges.RemovedResources,
			changes.ResourceName,
		)
	} else {
		if c.outputChanges.ResourceChanges == nil {
			c.outputChanges.ResourceChanges = map[string]*provider.Changes{}
		}
		c.outputChanges.ResourceChanges[changes.ResourceName] = &changes.Changes
	}

	c.outputChanges.ResolveOnDeploy = append(
		c.outputChanges.ResolveOnDeploy,
		commoncore.Map(
			changes.Changes.FieldChangesKnownOnDeploy,
			toFullResourcePath(changes.ResourceName),
		)...,
	)
}

func (c *defaultChangeStagingState) UpdateLinkStagingState(
	node *links.ChainLinkNode,
) []*LinkPendingCompletion {
	c.mu.Lock()
	defer c.mu.Unlock()

	hasLinks := len(node.LinksTo) > 0 || len(node.LinkedFrom) > 0
	pendingLinkNames := c.resourceNameLinkMap[node.ResourceName]
	if hasLinks {
		c.addPendingLinksToStagingState(node, pendingLinkNames)
	}
	return c.updatePendingLinksInStagingState(node, pendingLinkNames)
}

// This must only be called when a lock has already been held on the staging state.
func (c *defaultChangeStagingState) addPendingLinksToStagingState(
	node *links.ChainLinkNode,
	alreadyPendingLinks []string,
) {
	for _, linksToNode := range node.LinksTo {
		linkName := createLogicalLinkName(node.ResourceName, linksToNode.ResourceName)
		if !slices.Contains(alreadyPendingLinks, linkName) {
			completionState := &LinkPendingCompletion{
				resourceANode:    node,
				resourceBNode:    linksToNode,
				resourceAPending: false,
				resourceBPending: true,
				linkPending:      true,
			}
			c.pendingLinks[linkName] = completionState
			c.resourceNameLinkMap[node.ResourceName] = append(
				c.resourceNameLinkMap[node.ResourceName],
				linkName,
			)
			c.resourceNameLinkMap[linksToNode.ResourceName] = append(
				c.resourceNameLinkMap[linksToNode.ResourceName],
				linkName,
			)
		}
	}

	for _, linkedFromNode := range node.LinkedFrom {
		linkName := createLogicalLinkName(linkedFromNode.ResourceName, node.ResourceName)
		if !slices.Contains(alreadyPendingLinks, linkName) {
			completionState := &LinkPendingCompletion{
				resourceANode:    linkedFromNode,
				resourceBNode:    node,
				resourceAPending: true,
				resourceBPending: false,
				linkPending:      true,
			}
			c.pendingLinks[linkName] = completionState
			c.resourceNameLinkMap[linkedFromNode.ResourceName] = append(
				c.resourceNameLinkMap[linkedFromNode.ResourceName],
				linkName,
			)
			c.resourceNameLinkMap[node.ResourceName] = append(
				c.resourceNameLinkMap[node.ResourceName],
				linkName,
			)
		}
	}
}

// This must only be called when a lock has already been held on the staging state.
func (c *defaultChangeStagingState) updatePendingLinksInStagingState(
	node *links.ChainLinkNode,
	pendingLinkNames []string,
) []*LinkPendingCompletion {
	linksReadyToBeStaged := []*LinkPendingCompletion{}

	for _, linkName := range pendingLinkNames {
		completionState := c.pendingLinks[linkName]
		if completionState.resourceANode.ResourceName == node.ResourceName {
			completionState.resourceAPending = false
		} else if completionState.resourceBNode.ResourceName == node.ResourceName {
			completionState.resourceBPending = false
		}

		if !completionState.resourceAPending && !completionState.resourceBPending {
			linksReadyToBeStaged = append(linksReadyToBeStaged, completionState)
		}
	}

	return linksReadyToBeStaged
}

func (c *defaultChangeStagingState) MustRecreateResourceOnRemovedDependencies(
	resourceName string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, element := range c.mustRecreate.Resources {
		if element.ResourceName == resourceName {
			return true
		}
	}

	return false
}

func (c *defaultChangeStagingState) CountPendingLinksForGroup(group []*DeploymentNode) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for _, node := range group {
		if node.Type() == "resource" {
			pendingLinkNames := c.resourceNameLinkMap[node.ChainLinkNode.ResourceName]
			for _, linkName := range pendingLinkNames {
				if c.pendingLinks[linkName].linkPending {
					count += 1
				}
			}
		}
	}

	return count
}

func (c *defaultChangeStagingState) ApplyLinkChanges(changes LinkChangesMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if changes.Removed {
		c.outputChanges.RemovedLinks = append(
			c.outputChanges.RemovedLinks,
			createLogicalLinkName(changes.ResourceAName, changes.ResourceBName),
		)
		return
	}

	resourceChanges := getResourceChanges(changes.ResourceAName, c.outputChanges)
	if resourceChanges != nil {
		if changes.New {
			if resourceChanges.NewOutboundLinks == nil {
				resourceChanges.NewOutboundLinks = map[string]provider.LinkChanges{}
			}
			resourceChanges.NewOutboundLinks[changes.ResourceBName] = changes.Changes
		} else {
			if resourceChanges.OutboundLinkChanges == nil {
				resourceChanges.OutboundLinkChanges = map[string]provider.LinkChanges{}
			}
			resourceChanges.OutboundLinkChanges[changes.ResourceBName] = changes.Changes
		}
		c.outputChanges.ResolveOnDeploy = append(
			c.outputChanges.ResolveOnDeploy,
			commoncore.Map(
				changes.Changes.FieldChangesKnownOnDeploy,
				toFullLinkPath(changes.ResourceAName, changes.ResourceBName),
			)...,
		)
	}
}

func (c *defaultChangeStagingState) ApplyChildChanges(changes ChildChangesMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if changes.New {
		if c.outputChanges.NewChildren == nil {
			c.outputChanges.NewChildren = map[string]*NewBlueprintDefinition{}
		}

		c.outputChanges.NewChildren[changes.ChildBlueprintName] = &NewBlueprintDefinition{
			NewResources: changes.Changes.NewResources,
			NewChildren:  changes.Changes.NewChildren,
			NewExports:   changes.Changes.NewExports,
		}
	} else if changes.Removed {
		c.outputChanges.RemovedChildren = append(
			c.outputChanges.RemovedChildren,
			changes.ChildBlueprintName,
		)
	} else {
		if c.outputChanges.ChildChanges == nil {
			c.outputChanges.ChildChanges = map[string]*BlueprintChanges{}
		}
		c.outputChanges.ChildChanges[changes.ChildBlueprintName] = &changes.Changes
	}
}

func (c *defaultChangeStagingState) GetResourceChanges(resourceName string) *provider.Changes {
	c.mu.Lock()
	defer c.mu.Unlock()

	return getResourceChanges(resourceName, c.outputChanges)
}

func (c *defaultChangeStagingState) MarkLinkAsNoLongerPending(
	resourceANode, resourceBNode *links.ChainLinkNode,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	linkName := createLogicalLinkName(resourceANode.ResourceName, resourceBNode.ResourceName)
	pendingLink := c.pendingLinks[linkName]
	pendingLink.linkPending = false
}

func (c *defaultChangeStagingState) UpdateExportChanges(
	collectedExportChanges *intermediaryBlueprintChanges,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.outputChanges.NewExports = collectedExportChanges.NewExports
	c.outputChanges.ExportChanges = collectedExportChanges.ExportChanges
	c.outputChanges.UnchangedExports = collectedExportChanges.UnchangedExports
	c.outputChanges.RemovedExports = collectedExportChanges.RemovedExports
	c.outputChanges.ResolveOnDeploy = append(
		c.outputChanges.ResolveOnDeploy,
		collectedExportChanges.ResolveOnDeploy...,
	)
}

func (c *defaultChangeStagingState) ExtractBlueprintChanges() BlueprintChanges {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get children that must be recreated due to removed dependencies and remove
	// from child changes if present in child changes map.
	recreateChildren := c.collectChildrenToRecreate()

	return BlueprintChanges{
		NewResources:     copyPointerMap(c.outputChanges.NewResources),
		ResourceChanges:  copyPointerMap(c.outputChanges.ResourceChanges),
		RemovedResources: c.outputChanges.RemovedResources,
		RemovedLinks:     c.outputChanges.RemovedLinks,
		NewChildren:      copyPointerMap(c.outputChanges.NewChildren),
		RecreateChildren: recreateChildren,
		ChildChanges:     copyPointerMap(c.outputChanges.ChildChanges),
		RemovedChildren:  c.outputChanges.RemovedChildren,
		NewExports:       copyPointerMap(c.outputChanges.NewExports),
		ExportChanges:    copyPointerMap(c.outputChanges.ExportChanges),
		RemovedExports:   c.outputChanges.RemovedExports,
		ResolveOnDeploy:  c.outputChanges.ResolveOnDeploy,
	}
}

// A lock must be held on the staging state when calling this function.
func (c *defaultChangeStagingState) collectChildrenToRecreate() []string {
	recreateChildren := []string{}
	for _, child := range c.mustRecreate.Children {
		if c.outputChanges.ChildChanges[child.ChildName] != nil {
			recreateChildren = append(recreateChildren, child.ChildName)
		}
	}
	return recreateChildren
}

// A lock must be held on the staging state when calling this function.
func getResourceChanges(resourceName string, changes *intermediaryBlueprintChanges) *provider.Changes {

	newResourceChanges, hasNewResourceChanges := changes.NewResources[resourceName]
	if hasNewResourceChanges {
		return newResourceChanges
	}

	resourceChanges, hasResourceChanges := changes.ResourceChanges[resourceName]
	if hasResourceChanges {
		return resourceChanges
	}

	return nil
}