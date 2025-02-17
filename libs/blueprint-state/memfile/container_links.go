package memfile

import (
	"context"
	"fmt"
	"sync"

	"github.com/spf13/afero"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type linksContainerImpl struct {
	links     map[string]*state.LinkState
	instances map[string]*state.InstanceState
	fs        afero.Fs
	persister *statePersister
	logger    core.Logger
	mu        *sync.RWMutex
}

func (c *linksContainerImpl) Get(
	ctx context.Context,
	linkID string,
) (state.LinkState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if linkState, ok := c.links[linkID]; ok {
		return copyLink(linkState), nil
	}

	return state.LinkState{}, state.LinkNotFoundError(linkID)
}

func (c *linksContainerImpl) GetByName(
	ctx context.Context,
	instanceID string,
	linkName string,
) (state.LinkState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if instance, ok := getInstance(c.instances, instanceID); ok {
		if linkState, ok := instance.Links[linkName]; ok {
			return copyLink(linkState), nil
		}
	}

	elementID := fmt.Sprintf("instance:%s:link:%s", instanceID, linkName)
	return state.LinkState{}, state.LinkNotFoundError(elementID)
}

func (c *linksContainerImpl) Save(
	ctx context.Context,
	linkState state.LinkState,
) error {
	linkLogger := c.logger.WithFields(
		core.StringLogField("linkId", linkState.LinkID),
		core.StringLogField("instanceId", linkState.InstanceID),
		core.StringLogField("linkName", linkState.Name),
	)
	c.mu.Lock()
	defer c.mu.Unlock()

	if instance, ok := getInstance(c.instances, linkState.InstanceID); ok {
		instance.Links[linkState.Name] = &linkState
		c.links[linkState.LinkID] = &linkState

		linkLogger.Debug("persisting updated or newly created link")
		return c.persister.updateInstance(instance)
	}

	return state.InstanceNotFoundError(linkState.InstanceID)
}

func (c *linksContainerImpl) UpdateStatus(
	ctx context.Context,
	linkID string,
	statusInfo state.LinkStatusInfo,
) error {
	linkLogger := c.logger.WithFields(
		core.StringLogField("linkId", linkID),
	)

	c.mu.Lock()
	defer c.mu.Unlock()

	link, hasLink := c.links[linkID]
	if !hasLink {
		return state.LinkNotFoundError(linkID)
	}

	instance, ok := c.instances[link.InstanceID]
	if !ok {
		// When a link exists but the instance does not,
		// then something has corrupted the state.
		return errMalformedState(
			instanceNotFoundForLinkMessage(link.InstanceID, linkID),
		)
	}

	link.Status = statusInfo.Status
	link.PreciseStatus = statusInfo.PreciseStatus
	link.FailureReasons = statusInfo.FailureReasons
	if statusInfo.LastDeployAttemptTimestamp != nil {
		link.LastDeployAttemptTimestamp = *statusInfo.LastDeployAttemptTimestamp
	}
	if statusInfo.LastDeployedTimestamp != nil {
		link.LastDeployedTimestamp = *statusInfo.LastDeployedTimestamp
	}
	if statusInfo.LastStatusUpdateTimestamp != nil {
		link.LastStatusUpdateTimestamp = *statusInfo.LastStatusUpdateTimestamp
	}
	if statusInfo.Durations != nil {
		link.Durations = statusInfo.Durations
	}

	linkLogger.Debug("persisting link status update")
	return c.persister.updateInstance(instance)
}

func (c *linksContainerImpl) Remove(
	ctx context.Context,
	linkID string,
) (state.LinkState, error) {
	linkLogger := c.logger.WithFields(
		core.StringLogField("linkId", linkID),
	)

	c.mu.Lock()
	defer c.mu.Unlock()

	link, ok := c.links[linkID]
	if !ok {
		return state.LinkState{}, state.LinkNotFoundError(linkID)
	}

	instance, hasInstance := c.instances[link.InstanceID]
	if !hasInstance {
		// When a link exists but the instance does not,
		// then something has corrupted the state.
		return state.LinkState{}, errMalformedState(
			instanceNotFoundForLinkMessage(link.InstanceID, linkID),
		)
	}

	delete(instance.Links, link.Name)
	delete(c.links, linkID)

	linkLogger.Debug("persisting link removal")
	return *link, c.persister.updateInstance(instance)
}

func copyLink(linkState *state.LinkState) state.LinkState {
	if linkState == nil {
		return state.LinkState{}
	}

	return state.LinkState{
		LinkID:                     linkState.LinkID,
		Name:                       linkState.Name,
		InstanceID:                 linkState.InstanceID,
		Status:                     linkState.Status,
		PreciseStatus:              linkState.PreciseStatus,
		LastDeployedTimestamp:      linkState.LastDeployedTimestamp,
		LastDeployAttemptTimestamp: linkState.LastDeployAttemptTimestamp,
		LastStatusUpdateTimestamp:  linkState.LastStatusUpdateTimestamp,
		IntermediaryResourceStates: copyIntermediaryResources(
			linkState.IntermediaryResourceStates,
		),
		Data:           linkState.Data,
		FailureReasons: linkState.FailureReasons,
		Durations:      linkState.Durations,
	}
}

func copyIntermediaryResources(
	intermediaryResourceStates []*state.LinkIntermediaryResourceState,
) []*state.LinkIntermediaryResourceState {
	if intermediaryResourceStates == nil {
		return nil
	}

	intermediaryResourcesCopy := []*state.LinkIntermediaryResourceState{}
	for i, value := range intermediaryResourceStates {
		intermediaryResourcesCopy[i] = &state.LinkIntermediaryResourceState{
			ResourceID:                 value.ResourceID,
			InstanceID:                 value.InstanceID,
			LastDeployedTimestamp:      value.LastDeployedTimestamp,
			LastDeployAttemptTimestamp: value.LastDeployAttemptTimestamp,
			ResourceSpecData:           value.ResourceSpecData,
		}
	}

	return intermediaryResourcesCopy
}

func instanceNotFoundForLinkMessage(
	instanceID string,
	linkID string,
) string {
	return fmt.Sprintf("instance %s not found for link %s", instanceID, linkID)
}
