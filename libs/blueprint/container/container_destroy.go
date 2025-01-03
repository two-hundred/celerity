package container

import (
	"context"
	"strings"
	"time"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

const (
	prepareDestroyFailureMessage = "failed to load instance state while preparing to destroy"
)

func (c *defaultBlueprintContainer) Destroy(
	ctx context.Context,
	input *DestroyInput,
	channels *DeployChannels,
	paramOverrides core.BlueprintParams,
) {
	ctxWithInstanceID := context.WithValue(ctx, core.BlueprintInstanceIDKey, input.InstanceID)
	state := c.createDeploymentState()
	go c.destroy(
		ctxWithInstanceID,
		input,
		channels,
		state,
		paramOverrides,
	)
}

func (c *defaultBlueprintContainer) destroy(
	ctx context.Context,
	input *DestroyInput,
	channels *DeployChannels,
	state DeploymentState,
	paramOverrides core.BlueprintParams,
) {
	instanceTreePath := getInstanceTreePath(paramOverrides, input.InstanceID)
	if exceedsMaxDepth(instanceTreePath, MaxBlueprintDepth) {
		channels.ErrChan <- errMaxBlueprintDepthExceeded(
			instanceTreePath,
			MaxBlueprintDepth,
		)
		return
	}

	if input.Changes == nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			determineInstanceDestroyFailedStatus(input.Rollback),
			[]string{
				emptyChangesDestroyFailedMessage(input.Rollback),
			},
			/* elapsedTime */ 0,
			/* prepareElapsedTime */ nil,
		)
		return
	}

	startTime := c.clock.Now()
	channels.DeploymentUpdateChan <- DeploymentUpdateMessage{
		InstanceID:      input.InstanceID,
		Status:          determineInstanceDestroyingStatus(input.Rollback),
		UpdateTimestamp: startTime.Unix(),
	}

	instances := c.stateContainer.Instances()
	currentInstanceState, err := instances.Get(ctx, input.InstanceID)
	if err != nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			determineInstanceDestroyFailedStatus(input.Rollback),
			[]string{prepareDestroyFailureMessage},
			c.clock.Since(startTime),
			/* prepareElapsedTime */ nil,
		)
		return
	}

	resourceProviderMap := c.resourceProviderMapFromState(&currentInstanceState)

	sentFinishedMessage, err := c.removeElements(
		ctx,
		&DeployInput{
			InstanceID: input.InstanceID,
			Changes:    input.Changes,
			Rollback:   input.Rollback,
		},
		&DeployContext{
			StartTime:             startTime,
			State:                 state,
			Rollback:              input.Rollback,
			Destroying:            true,
			Channels:              channels,
			ParamOverrides:        paramOverrides,
			InstanceStateSnapshot: &currentInstanceState,
			ResourceProviders:     resourceProviderMap,
		},
		[]*DeploymentNode{},
	)
	if err != nil {
		channels.ErrChan <- wrapErrorForChildContext(err, paramOverrides)
		return
	}

	if sentFinishedMessage {
		return
	}

	sentFinishedMessage = c.removeBlueprintInstanceFromState(ctx, input, channels, startTime, instances)
	if sentFinishedMessage {
		return
	}

	channels.FinishChan <- c.createDeploymentFinishedMessage(
		input.InstanceID,
		determineInstanceDestroyedStatus(input.Rollback),
		[]string{},
		c.clock.Since(startTime),
		/* prepareElapsedTime */ nil,
	)
}

func (c *defaultBlueprintContainer) removeBlueprintInstanceFromState(
	ctx context.Context,
	input *DestroyInput,
	channels *DeployChannels,
	startTime time.Time,
	instances state.InstancesContainer,
) bool {
	_, err := instances.Remove(ctx, input.InstanceID)
	if err != nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			determineInstanceDestroyFailedStatus(input.Rollback),
			[]string{err.Error()},
			c.clock.Since(startTime),
			/* prepareElapsedTime */ nil,
		)
		return true
	}

	return false
}

func (c *defaultBlueprintContainer) resourceProviderMapFromState(
	currentInstanceState *state.InstanceState,
) map[string]provider.Provider {
	resourceProviderMap := map[string]provider.Provider{}
	for _, resourceState := range currentInstanceState.Resources {
		providerNamespace := strings.Split(resourceState.ResourceType, "/")[0]
		resourceProviderMap[resourceState.ResourceName] = c.providers[providerNamespace]
	}
	return resourceProviderMap
}

func (c *defaultBlueprintContainer) removeElements(
	ctx context.Context,
	input *DeployInput,
	deployCtx *DeployContext,
	nodesToBeDeployed []*DeploymentNode,
) (bool, error) {

	elementsToRemove, finished, err := c.collectElementsToRemove(
		input.Changes,
		deployCtx,
		nodesToBeDeployed,
	)
	if err != nil {
		return true, err
	}

	if finished {
		return true, nil
	}

	orderedElements := OrderElementsForRemoval(
		elementsToRemove,
		deployCtx.InstanceStateSnapshot,
	)
	groupedElements := GroupOrderedElementsForRemoval(orderedElements)

	if !deployCtx.Destroying {
		// Stash the prepare duration here for both destroy and deploy as where there are
		// elements to be removed, they will always be processed first, this allows us to more
		// accurately track duration as the prepare phase is complete once the elements to be
		// removed have been collected, ordered and grouped.
		// In the case where there are no elements to be removed, this will still be called
		// for a deployment, as removal of existing elements is always processed first,
		// this is a reliable way to track the prepare duration and send the status change.
		deployCtx.State.SetPrepareDuration(c.clock.Since(deployCtx.StartTime))
		deployCtx.Channels.DeploymentUpdateChan <- DeploymentUpdateMessage{
			InstanceID:      input.InstanceID,
			Status:          determineInstanceDeployingStatus(input.Rollback, false /* newInstance */),
			UpdateTimestamp: c.clock.Now().Unix(),
		}
	}

	stopProcessing, err := c.removeGroupedElements(
		ctx,
		groupedElements,
		input.InstanceID,
		deployCtx,
	)
	if err != nil {
		return stopProcessing, err
	}

	return stopProcessing, nil
}

func (c *defaultBlueprintContainer) removeGroupedElements(
	ctx context.Context,
	parallelGroups [][]state.Element,
	instanceID string,
	deployCtx *DeployContext,
) (bool, error) {
	internalChannels := CreateDeployChannels()

	stopProcessing := false
	i := 0
	var err error
	for !stopProcessing && i < len(parallelGroups) {
		group := parallelGroups[i]
		c.removeGroupElements(
			ctx,
			instanceID,
			group,
			DeployContextWithGroup(
				DeployContextWithChannels(deployCtx, internalChannels),
				i,
			),
		)

		stopProcessing, err = c.listenToAndProcessGroupRemovals(
			ctx,
			instanceID,
			group,
			deployCtx,
			internalChannels,
		)
		i += 1
	}

	return stopProcessing, err
}

func (c *defaultBlueprintContainer) listenToAndProcessGroupRemovals(
	ctx context.Context,
	instanceID string,
	group []state.Element,
	deployCtx *DeployContext,
	internalChannels *DeployChannels,
) (bool, error) {
	finished := map[string]*deployUpdateMessageWrapper{}

	var err error
	for (len(finished) < len(group)) &&
		err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case msg := <-internalChannels.ResourceUpdateChan:
			err = c.handleResourceUpdateMessage(ctx, instanceID, msg, deployCtx, finished)
		case msg := <-internalChannels.ChildUpdateChan:
			err = c.handleChildUpdateMessage(ctx, instanceID, msg, deployCtx, finished)
		case msg := <-internalChannels.LinkUpdateChan:
			err = c.handleLinkUpdateMessage(ctx, instanceID, msg, deployCtx, finished)
		case err = <-internalChannels.ErrChan:
		}
	}

	if err != nil {
		return true, err
	}

	failed := getFailedRemovalsAndUpdateState(finished, group, deployCtx.State, deployCtx.Rollback)
	if len(failed) > 0 {
		deployCtx.Channels.FinishChan <- c.createDeploymentFinishedMessage(
			instanceID,
			determineFinishedFailureStatus(deployCtx.Destroying, deployCtx.Rollback),
			finishedFailureMessages(deployCtx, failed),
			c.clock.Since(deployCtx.StartTime),
			/* prepareElapsedTime */
			deployCtx.State.GetPrepareDuration(),
		)
		return true, nil
	}

	return false, nil
}

func (c *defaultBlueprintContainer) handleResourceDestroyEvent(
	ctx context.Context,
	msg ResourceDeployUpdateMessage,
	deployCtx *DeployContext,
	finished map[string]*deployUpdateMessageWrapper,
	elementName string,
) error {
	resources := c.stateContainer.Resources()
	if startedDestroyingResource(msg.PreciseStatus, deployCtx.Rollback) {
		err := resources.UpdateStatus(
			ctx,
			msg.InstanceID,
			msg.ResourceID,
			state.ResourceStatusInfo{
				Status:        msg.Status,
				PreciseStatus: msg.PreciseStatus,
			},
		)
		if err != nil {
			return err
		}
	}

	if finishedDestroyingResource(msg, deployCtx.Rollback) {
		finished[elementName] = &deployUpdateMessageWrapper{
			resourceUpdateMessage: &msg,
		}

		if wasResourceDestroyedSuccessfully(msg.PreciseStatus, deployCtx.Rollback) {
			_, err := resources.Remove(
				ctx,
				msg.InstanceID,
				msg.ResourceID,
			)
			if err != nil {
				return err
			}
		} else {
			err := resources.UpdateStatus(
				ctx,
				msg.InstanceID,
				msg.ResourceID,
				state.ResourceStatusInfo{
					Status:         msg.Status,
					PreciseStatus:  msg.PreciseStatus,
					FailureReasons: msg.FailureReasons,
					Durations:      msg.Durations,
				},
			)
			if err != nil {
				return err
			}
		}
	}

	deployCtx.Channels.ResourceUpdateChan <- msg
	return nil
}

func (c *defaultBlueprintContainer) handleChildDestroyEvent(
	ctx context.Context,
	msg ChildDeployUpdateMessage,
	deployCtx *DeployContext,
	finished map[string]*deployUpdateMessageWrapper,
	elementName string,
) error {
	instances := c.stateContainer.Instances()
	children := c.stateContainer.Children()
	if startedDestroyingChild(msg.Status, deployCtx.Rollback) {
		err := instances.UpdateStatus(
			ctx,
			msg.ChildInstanceID,
			state.InstanceStatusInfo{
				Status: msg.Status,
			},
		)
		if err != nil {
			return err
		}
	}

	if finishedDestroyingChild(msg, deployCtx.Rollback) {
		finished[elementName] = &deployUpdateMessageWrapper{
			childUpdateMessage: &msg,
		}

		if wasChildDestroyedSuccessfully(msg.Status, deployCtx.Rollback) {
			_, err := children.Remove(
				ctx,
				msg.ParentInstanceID,
				msg.ChildName,
			)
			if err != nil {
				return err
			}
		} else {
			err := instances.UpdateStatus(
				ctx,
				msg.ChildInstanceID,
				state.InstanceStatusInfo{
					Status:    msg.Status,
					Durations: msg.Durations,
				},
			)
			if err != nil {
				return err
			}
		}
	}

	deployCtx.Channels.ChildUpdateChan <- msg
	return nil
}

func (c *defaultBlueprintContainer) handleLinkDestroyEvent(
	ctx context.Context,
	msg LinkDeployUpdateMessage,
	deployCtx *DeployContext,
	finished map[string]*deployUpdateMessageWrapper,
	elementName string,
) error {
	links := c.stateContainer.Links()
	if startedDestroyingLink(msg.Status, deployCtx.Rollback) {
		err := links.UpdateStatus(
			ctx,
			msg.InstanceID,
			msg.LinkID,
			state.LinkStatusInfo{
				Status:        msg.Status,
				PreciseStatus: msg.PreciseStatus,
				// For links, there are multiple stages to the destroy process,
				// a status update for each stage will contain
				// duration information for the previous stages.
				Durations: msg.Durations,
			},
		)
		if err != nil {
			return err
		}
	}

	if finishedDestroyingLink(msg, deployCtx.Rollback) {
		finished[elementName] = &deployUpdateMessageWrapper{
			linkUpdateMessage: &msg,
		}

		if wasLinkDestroyedSuccessfully(msg.Status, deployCtx.Rollback) {
			_, err := links.Remove(
				ctx,
				msg.InstanceID,
				msg.LinkID,
			)
			if err != nil {
				return err
			}
		} else {
			err := links.UpdateStatus(
				ctx,
				msg.InstanceID,
				msg.LinkID,
				state.LinkStatusInfo{
					Status:         msg.Status,
					PreciseStatus:  msg.PreciseStatus,
					FailureReasons: msg.FailureReasons,
					Durations:      msg.Durations,
				},
			)
			if err != nil {
				return err
			}
		}
	}

	deployCtx.Channels.LinkUpdateChan <- msg
	return nil
}

func (c *defaultBlueprintContainer) removeGroupElements(
	ctx context.Context,
	instanceID string,
	group []state.Element,
	deployCtx *DeployContext,
) {
	instanceTreePath := getInstanceTreePath(deployCtx.ParamOverrides, instanceID)

	for _, element := range group {
		if element.Kind() == state.ResourceElement {
			go c.resourceDestroyer.Destroy(
				ctx,
				element,
				instanceID,
				deployCtx,
			)
		} else if element.Kind() == state.ChildElement {
			includeTreePath := getIncludeTreePath(deployCtx.ParamOverrides, element.LogicalName())
			go c.childBlueprintDestroyer.Destroy(
				ctx,
				element,
				instanceID,
				instanceTreePath,
				includeTreePath,
				c,
				deployCtx,
			)
		} else if element.Kind() == state.LinkElement {
			go c.prepareAndDestroyLink(
				ctx,
				element,
				instanceID,
				deployCtx,
			)
		}
	}
}

func (c *defaultBlueprintContainer) prepareAndDestroyLink(
	ctx context.Context,
	linkElement state.Element,
	instanceID string,
	deployCtx *DeployContext,
) {
	linkState := getLinkStateByName(
		deployCtx.InstanceStateSnapshot,
		linkElement.LogicalName(),
	)
	if linkState == nil {
		deployCtx.Channels.ErrChan <- errLinkNotFoundInState(
			linkElement.LogicalName(),
			instanceID,
		)
		return
	}

	linkImplementation, err := c.getProviderLinkImplementation(
		ctx,
		linkElement.LogicalName(),
		deployCtx.InstanceStateSnapshot,
	)
	if err != nil {
		deployCtx.Channels.ErrChan <- err
		return
	}

	retryPolicy, err := c.getLinkRetryPolicy(
		ctx,
		linkElement.LogicalName(),
		deployCtx.InstanceStateSnapshot,
	)
	if err != nil {
		deployCtx.Channels.ErrChan <- err
		return
	}

	err = c.destroyLink(
		ctx,
		&deploymentElementInfo{
			element:    linkElement,
			instanceID: instanceID,
		},
		linkImplementation,
		deployCtx,
		retryPolicy,
	)
	if err != nil {
		deployCtx.Channels.ErrChan <- err
	}
}

func (c *defaultBlueprintContainer) destroyLink(
	ctx context.Context,
	linkInfo *deploymentElementInfo,
	linkImplementation provider.Link,
	deployCtx *DeployContext,
	retryPolicy *provider.RetryPolicy,
) error {
	linkDependencyInfo := extractLinkDirectDependencies(
		linkInfo.element.LogicalName(),
	)

	resourceAInfo := getResourceInfoFromStateForLinkRemoval(
		deployCtx.InstanceStateSnapshot,
		linkDependencyInfo.resourceAName,
	)
	_, stop, err := c.updateLinkResourceA(
		ctx,
		linkImplementation,
		&provider.LinkUpdateResourceInput{
			ResourceInfo:   resourceAInfo,
			LinkUpdateType: provider.LinkUpdateTypeDestroy,
			Params:         deployCtx.ParamOverrides,
		},
		linkInfo,
		createRetryInfo(retryPolicy),
		deployCtx,
	)
	if err != nil {
		return err
	}
	if stop {
		return nil
	}

	resourceBInfo := getResourceInfoFromStateForLinkRemoval(
		deployCtx.InstanceStateSnapshot,
		linkDependencyInfo.resourceBName,
	)
	_, stop, err = c.updateLinkResourceB(
		ctx,
		linkImplementation,
		&provider.LinkUpdateResourceInput{
			ResourceInfo:   resourceBInfo,
			LinkUpdateType: provider.LinkUpdateTypeDestroy,
			Params:         deployCtx.ParamOverrides,
		},
		linkInfo,
		createRetryInfo(retryPolicy),
		deployCtx,
	)
	if err != nil {
		return err
	}
	if stop {
		return nil
	}

	_, err = c.updateLinkIntermediaryResources(
		ctx,
		linkImplementation,
		&provider.LinkUpdateIntermediaryResourcesInput{
			ResourceAInfo:  resourceAInfo,
			ResourceBInfo:  resourceBInfo,
			LinkUpdateType: provider.LinkUpdateTypeDestroy,
			Params:         deployCtx.ParamOverrides,
		},
		linkInfo,
		createRetryInfo(retryPolicy),
		deployCtx,
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *defaultBlueprintContainer) getProviderLinkImplementation(
	ctx context.Context,
	linkName string,
	currentState *state.InstanceState,
) (provider.Link, error) {

	resourceTypeA, resourceTypeB, err := getResourceTypesForLink(linkName, currentState)
	if err != nil {
		return nil, err
	}

	return c.linkRegistry.Link(ctx, resourceTypeA, resourceTypeB)
}

func (c *defaultBlueprintContainer) collectElementsToRemove(
	changes *BlueprintChanges,
	deployCtx *DeployContext,
	nodesToBeDeployed []*DeploymentNode,
) (*CollectedElements, bool, error) {
	if len(changes.RemovedChildren) == 0 &&
		len(changes.RemovedResources) == 0 &&
		len(changes.RemovedLinks) == 0 {
		return &CollectedElements{}, false, nil
	}

	resourcesToRemove, err := c.collectResourcesToRemove(
		deployCtx.InstanceStateSnapshot,
		changes,
		nodesToBeDeployed,
	)
	if err != nil {
		message := getDeploymentErrorSpecificMessage(err, prepareFailureMessage)
		deployCtx.Channels.FinishChan <- c.createDeploymentFinishedMessage(
			deployCtx.InstanceStateSnapshot.InstanceID,
			determineFinishedFailureStatus(deployCtx.Destroying, deployCtx.Rollback),
			[]string{message},
			c.clock.Since(deployCtx.StartTime),
			/* prepareElapsedTime */ nil,
		)
		return &CollectedElements{}, true, nil
	}

	childrenToRemove, err := c.collectChildrenToRemove(
		deployCtx.InstanceStateSnapshot,
		changes,
		nodesToBeDeployed,
	)
	if err != nil {
		message := getDeploymentErrorSpecificMessage(err, prepareFailureMessage)
		deployCtx.Channels.FinishChan <- c.createDeploymentFinishedMessage(
			deployCtx.InstanceStateSnapshot.InstanceID,
			determineFinishedFailureStatus(deployCtx.Destroying, deployCtx.Rollback),
			[]string{message},
			c.clock.Since(deployCtx.StartTime),
			/* prepareElapsedTime */ nil,
		)
		return &CollectedElements{}, true, nil
	}

	linksToRemove := c.collectLinksToRemove(deployCtx.InstanceStateSnapshot, changes)

	return &CollectedElements{
		Resources: resourcesToRemove,
		Children:  childrenToRemove,
		Links:     linksToRemove,
		Total:     len(resourcesToRemove) + len(childrenToRemove) + len(linksToRemove),
	}, false, nil
}

func (c *defaultBlueprintContainer) collectResourcesToRemove(
	currentState *state.InstanceState,
	changes *BlueprintChanges,
	nodesToBeDeployed []*DeploymentNode,
) ([]*ResourceIDInfo, error) {
	resourcesToRemove := []*ResourceIDInfo{}
	for _, resourceToRemove := range changes.RemovedResources {
		toBeRemovedResourceState := getResourceStateByName(currentState, resourceToRemove)
		if toBeRemovedResourceState != nil {
			// Check if the resource has dependents that will not be removed or recreated.
			// Resources that previously depended on the resource to be removed
			// and are marked to be recreated will no longer have a dependency on the resource
			// in question. This is because the same logic is applied during change staging
			// to mark a resource or child blueprint to be recreated if
			// it previously depended on a resource that is being removed.
			elements := filterOutRecreated(
				// For this purpose, there is no need to check transitive dependencies
				// as for a transitive dependency to exist, a direct dependency would also need to exist
				// and as soon as a direct dependency is found that will not be removed or recreated,
				// the deployment process will be stopped.
				findDependents(toBeRemovedResourceState, nodesToBeDeployed, currentState),
				changes,
			)
			if elements.Total > 0 {
				return nil, errResourceToBeRemovedHasDependents(resourceToRemove, elements)
			}
			resourcesToRemove = append(resourcesToRemove, &ResourceIDInfo{
				ResourceID:   toBeRemovedResourceState.ResourceID,
				ResourceName: toBeRemovedResourceState.ResourceName,
			})
		}
	}
	return resourcesToRemove, nil
}

func (c *defaultBlueprintContainer) collectChildrenToRemove(
	currentState *state.InstanceState,
	changes *BlueprintChanges,
	nodesToBeDeployed []*DeploymentNode,
) ([]*ChildBlueprintIDInfo, error) {
	childrenToRemove := []*ChildBlueprintIDInfo{}
	// Child blueprints that are marked to be recreated will need to be removed an addition
	// to those that have been removed from the source blueprint.
	combinedChildrenToRemove := append(changes.RemovedChildren, changes.RecreateChildren...)
	for _, childToRemove := range combinedChildrenToRemove {
		toBeRemovedChildState := getChildStateByName(currentState, childToRemove)
		if toBeRemovedChildState != nil {
			elements := filterOutRecreated(
				findDependents(
					state.WrapChildBlueprintInstance(childToRemove, toBeRemovedChildState),
					nodesToBeDeployed,
					currentState,
				),
				changes,
			)
			if elements.Total > 0 {
				return nil, errChildToBeRemovedHasDependents(childToRemove, elements)
			}
			childrenToRemove = append(childrenToRemove, &ChildBlueprintIDInfo{
				ChildInstanceID: toBeRemovedChildState.InstanceID,
				ChildName:       childToRemove,
			})
		}
	}
	return childrenToRemove, nil
}

func (c *defaultBlueprintContainer) collectLinksToRemove(
	currentState *state.InstanceState,
	changes *BlueprintChanges,
) []*LinkIDInfo {
	linksToRemove := []*LinkIDInfo{}
	for _, linkToRemove := range changes.RemovedLinks {
		toBeRemovedLinkState := getLinkStateByName(currentState, linkToRemove)
		if toBeRemovedLinkState != nil {
			linksToRemove = append(linksToRemove, &LinkIDInfo{
				LinkID:   toBeRemovedLinkState.LinkID,
				LinkName: linkToRemove,
			})
		}
	}
	return linksToRemove
}
