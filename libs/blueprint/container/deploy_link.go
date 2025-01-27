package container

import (
	"context"
	"time"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

// LinkDeployer provides an interface for a service that deploys a link between two
// resources as a part of the deployment process for a blueprint instance.
// This can be used for creating, updating and deleting a link between two resources.
// "Deploying" a link in the context of destruction means detaching information
// saved in the 2 resources related to the link and the removal of any intermediary
// resources created by a provider link implementation.
type LinkDeployer interface {
	Deploy(
		ctx context.Context,
		linkElement state.Element,
		instanceID string,
		linkUpdateType provider.LinkUpdateType,
		linkImplementation provider.Link,
		deployCtx *DeployContext,
		retryPolicy *provider.RetryPolicy,
	) error
}

// LinkDeployResult contains the result of deploying a link between two resources
// in a blueprint instance.
// LinkData contains the merged data from the link update operations on the two resources
// and intermediary resources.
type LinkDeployResult struct {
	IntermediaryResourceStates []*state.LinkIntermediaryResourceState
	LinkData                   *core.MappingNode
}

// NewDefaultLinkDeployer creates a new instance of the default implementation
// of the service that deploys a link between two resources as a part of the deployment process
// for a blueprint instance.
func NewDefaultLinkDeployer(clock core.Clock, stateContainer state.Container) LinkDeployer {
	return &defaultLinkDeployer{
		clock:          clock,
		stateContainer: stateContainer,
	}
}

type defaultLinkDeployer struct {
	clock          core.Clock
	stateContainer state.Container
}

func (d *defaultLinkDeployer) Deploy(
	ctx context.Context,
	linkElement state.Element,
	instanceID string,
	linkUpdateType provider.LinkUpdateType,
	linkImplementation provider.Link,
	deployCtx *DeployContext,
	retryPolicy *provider.RetryPolicy,
) error {
	linkDependencyInfo := extractLinkDirectDependencies(
		linkElement.LogicalName(),
	)

	resourceAInfo := getResourceInfoFromStateForLinkDeployment(
		deployCtx.InstanceStateSnapshot,
		linkDependencyInfo.resourceAName,
	)
	resourceBInfo := getResourceInfoFromStateForLinkDeployment(
		deployCtx.InstanceStateSnapshot,
		linkDependencyInfo.resourceBName,
	)

	if linkUpdateType == provider.LinkUpdateTypeCreate {
		links := d.stateContainer.Links()
		err := links.Save(
			ctx,
			instanceID,
			state.LinkState{
				LinkID:        linkElement.ID(),
				LinkName:      linkElement.LogicalName(),
				InstanceID:    instanceID,
				Status:        core.LinkStatusUnknown,
				PreciseStatus: core.PreciseLinkStatusUnknown,
			},
		)
		if err != nil {
			return err
		}
	}

	linkInfo := &deploymentElementInfo{
		element:    linkElement,
		instanceID: instanceID,
	}
	linkCtx := provider.NewLinkContextFromParams(deployCtx.ParamOverrides)
	resourceAOutput, stop, err := d.updateLinkResourceA(
		ctx,
		linkImplementation,
		&provider.LinkUpdateResourceInput{
			ResourceInfo:      resourceAInfo,
			OtherResourceInfo: resourceBInfo,
			LinkUpdateType:    linkUpdateType,
			LinkContext:       linkCtx,
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

	resourceBOutput, stop, err := d.updateLinkResourceB(
		ctx,
		linkImplementation,
		&provider.LinkUpdateResourceInput{
			ResourceInfo:      resourceBInfo,
			OtherResourceInfo: resourceAInfo,
			LinkUpdateType:    linkUpdateType,
			LinkContext:       linkCtx,
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

	err = d.updateLinkIntermediaryResources(
		ctx,
		linkImplementation,
		&provider.LinkUpdateIntermediaryResourcesInput{
			ResourceAInfo:  resourceAInfo,
			ResourceBInfo:  resourceBInfo,
			LinkUpdateType: linkUpdateType,
			LinkContext:    linkCtx,
		},
		linkInfo,
		createRetryInfo(retryPolicy),
		resourceAOutput,
		resourceBOutput,
		deployCtx,
	)
	if err != nil {
		return err
	}

	return nil
}

func (d *defaultLinkDeployer) updateLinkResourceA(
	ctx context.Context,
	linkImplementation provider.Link,
	input *provider.LinkUpdateResourceInput,
	linkInfo *deploymentElementInfo,
	updateResourceARetryInfo *retryInfo,
	deployCtx *DeployContext,
) (*provider.LinkUpdateResourceOutput, bool, error) {
	updateResourceAStartTime := d.clock.Now()
	deployCtx.Channels.LinkUpdateChan <- d.createLinkUpdatingResourceAMessage(
		linkInfo,
		deployCtx,
		updateResourceARetryInfo,
		input.LinkUpdateType,
	)

	resourceAOutput, err := linkImplementation.UpdateResourceA(ctx, input)
	if err != nil {
		if provider.IsRetryableError(err) {
			retryErr := err.(*provider.RetryableError)
			return d.handleUpdateLinkResourceARetry(
				ctx,
				linkInfo,
				linkImplementation,
				updateResourceARetryInfo,
				updateResourceAStartTime,
				&linkUpdateResourceInfo{
					failureReasons: []string{retryErr.ChildError.Error()},
					input:          input,
				},
				deployCtx,
			)
		}

		if provider.IsLinkUpdateResourceAError(err) {
			linkUpdateResourceAError := err.(*provider.LinkUpdateResourceAError)
			stop, err := d.handleUpdateResourceATerminalFailure(
				linkInfo,
				updateResourceARetryInfo,
				updateResourceAStartTime,
				&linkUpdateResourceInfo{
					failureReasons: linkUpdateResourceAError.FailureReasons,
					input:          input,
				},
				deployCtx,
			)
			return nil, stop, err
		}

		// For errors that are not wrapped in a provider error, the error is assumed to be fatal
		// and the deployment process will be stopped without reporting a failure state.
		// It is really important that adequate guidance is provided for provider developers
		// to ensure that all errors are wrapped in the appropriate provider error.
		return nil, true, err
	}

	deployCtx.Channels.LinkUpdateChan <- d.createLinkResourceAUpdatedMessage(
		linkInfo,
		deployCtx,
		updateResourceARetryInfo,
		input.LinkUpdateType,
		updateResourceAStartTime,
	)

	return resourceAOutput, false, nil
}

func (d *defaultLinkDeployer) handleUpdateLinkResourceARetry(
	ctx context.Context,
	linkInfo *deploymentElementInfo,
	linkImplementation provider.Link,
	updateResourceARetryInfo *retryInfo,
	updateResourceAStartTime time.Time,
	updateInfo *linkUpdateResourceInfo,
	deployCtx *DeployContext,
) (*provider.LinkUpdateResourceOutput, bool, error) {
	currentAttemptDuration := d.clock.Since(updateResourceAStartTime)
	nextRetryInfo := addRetryAttempt(updateResourceARetryInfo, currentAttemptDuration)
	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkResourceAUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons: updateInfo.failureReasons,
		// Attempt and retry information included the status update is specific to
		// updating resource A, each component of a link change will have its own
		// number of attempts and retry information.
		CurrentStageAttempt:  updateResourceARetryInfo.attempt,
		CanRetryCurrentStage: !nextRetryInfo.exceededMaxRetries,
		UpdateTimestamp:      d.clock.Now().Unix(),
		// Attempt durations will be accumulated and sent in the status updates
		// for each subsequent retry.
		// Total duration will be calculated if retry limit is exceeded.
		Durations: determineLinkUpdateResourceARetryFailureDurations(
			nextRetryInfo,
		),
	}

	if !nextRetryInfo.exceededMaxRetries {
		waitTimeMS := provider.CalculateRetryWaitTimeMS(nextRetryInfo.policy, nextRetryInfo.attempt)
		time.Sleep(time.Duration(waitTimeMS) * time.Millisecond)
		return d.updateLinkResourceA(
			ctx,
			linkImplementation,
			updateInfo.input,
			linkInfo,
			nextRetryInfo,
			deployCtx,
		)
	}

	return nil, true, nil
}

func (d *defaultLinkDeployer) handleUpdateResourceATerminalFailure(
	linkInfo *deploymentElementInfo,
	updateResourceARetryInfo *retryInfo,
	updateResourceAStartTime time.Time,
	updateInfo *linkUpdateResourceInfo,
	deployCtx *DeployContext,
) (bool, error) {
	currentAttemptDuration := d.clock.Since(updateResourceAStartTime)
	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkResourceAUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons:      updateInfo.failureReasons,
		CurrentStageAttempt: updateResourceARetryInfo.attempt,
		UpdateTimestamp:     d.clock.Now().Unix(),
		Durations: determineLinkUpdateResourceAFinishedDurations(
			updateResourceARetryInfo,
			currentAttemptDuration,
		),
	}

	return true, nil
}

func (d *defaultLinkDeployer) createLinkUpdatingResourceAMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateResourceARetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
) LinkDeployUpdateMessage {
	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdatingStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus: determinePreciseLinkUpdatingResourceAStatus(
			deployCtx.Rollback,
		),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateResourceARetryInfo.attempt,
	}
}

func (d *defaultLinkDeployer) createLinkResourceAUpdatedMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateResourceARetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
	updateResourceAStartTime time.Time,
) LinkDeployUpdateMessage {
	durations := determineLinkUpdateResourceAFinishedDurations(
		updateResourceARetryInfo,
		d.clock.Since(updateResourceAStartTime),
	)
	linkName := linkInfo.element.LogicalName()
	deployCtx.State.SetLinkDurationInfo(linkName, durations)

	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkName,
		// We are still in the process of updating the link,
		// resource B and intermediary resources still need to be updated.
		Status: determineLinkUpdatingStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus:       determinePreciseLinkResourceAUpdatedStatus(deployCtx.Rollback),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateResourceARetryInfo.attempt,
		Durations:           durations,
	}
}

func (d *defaultLinkDeployer) updateLinkResourceB(
	ctx context.Context,
	linkImplementation provider.Link,
	input *provider.LinkUpdateResourceInput,
	linkInfo *deploymentElementInfo,
	updateResourceBRetryInfo *retryInfo,
	deployCtx *DeployContext,
) (*provider.LinkUpdateResourceOutput, bool, error) {
	updateResourceBStartTime := d.clock.Now()
	deployCtx.Channels.LinkUpdateChan <- d.createLinkUpdatingResourceBMessage(
		linkInfo,
		deployCtx,
		updateResourceBRetryInfo,
		input.LinkUpdateType,
	)

	resourceBOutput, err := linkImplementation.UpdateResourceB(ctx, input)
	if err != nil {
		if provider.IsRetryableError(err) {
			retryErr := err.(*provider.RetryableError)
			return d.handleUpdateLinkResourceBRetry(
				ctx,
				linkInfo,
				linkImplementation,
				updateResourceBRetryInfo,
				updateResourceBStartTime,
				&linkUpdateResourceInfo{
					failureReasons: []string{retryErr.ChildError.Error()},
					input:          input,
				},
				deployCtx,
			)
		}

		if provider.IsLinkUpdateResourceBError(err) {
			linkUpdateResourceBError := err.(*provider.LinkUpdateResourceBError)
			stop, err := d.handleUpdateResourceBTerminalFailure(
				linkInfo,
				updateResourceBRetryInfo,
				updateResourceBStartTime,
				&linkUpdateResourceInfo{
					failureReasons: linkUpdateResourceBError.FailureReasons,
					input:          input,
				},
				deployCtx,
			)
			return nil, stop, err
		}

		// For errors that are not wrapped in a provider error, the error is assumed to be fatal
		// and the deployment process will be stopped without reporting a failure state.
		// It is really important that adequate guidance is provided for provider developers
		// to ensure that all errors are wrapped in the appropriate provider error.
		return nil, true, err
	}

	deployCtx.Channels.LinkUpdateChan <- d.createLinkResourceBUpdatedMessage(
		linkInfo,
		deployCtx,
		updateResourceBRetryInfo,
		input.LinkUpdateType,
		updateResourceBStartTime,
	)

	return resourceBOutput, false, nil
}

func (d *defaultLinkDeployer) handleUpdateLinkResourceBRetry(
	ctx context.Context,
	linkInfo *deploymentElementInfo,
	linkImplementation provider.Link,
	updateResourceBRetryInfo *retryInfo,
	updateResourceBStartTime time.Time,
	updateInfo *linkUpdateResourceInfo,
	deployCtx *DeployContext,
) (*provider.LinkUpdateResourceOutput, bool, error) {
	currentAttemptDuration := d.clock.Since(updateResourceBStartTime)
	nextRetryInfo := addRetryAttempt(updateResourceBRetryInfo, currentAttemptDuration)
	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkResourceBUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons: updateInfo.failureReasons,
		// Attempt and retry information included the status update is specific to
		// updating resource B, each component of a link change will have its own
		// number of attempts and retry information.
		CurrentStageAttempt:  updateResourceBRetryInfo.attempt,
		CanRetryCurrentStage: !nextRetryInfo.exceededMaxRetries,
		UpdateTimestamp:      d.clock.Now().Unix(),
		// Attempt durations will be accumulated and sent in the status updates
		// for each subsequent retry.
		// Total duration will be calculated if retry limit is exceeded.
		Durations: determineLinkUpdateResourceBRetryFailureDurations(
			nextRetryInfo,
		),
	}

	if !nextRetryInfo.exceededMaxRetries {
		waitTimeMS := provider.CalculateRetryWaitTimeMS(nextRetryInfo.policy, nextRetryInfo.attempt)
		time.Sleep(time.Duration(waitTimeMS) * time.Millisecond)
		return d.updateLinkResourceB(
			ctx,
			linkImplementation,
			updateInfo.input,
			linkInfo,
			nextRetryInfo,
			deployCtx,
		)
	}

	return nil, true, nil
}

func (d *defaultLinkDeployer) handleUpdateResourceBTerminalFailure(
	linkInfo *deploymentElementInfo,
	updateResourceBRetryInfo *retryInfo,
	updateResourceBStartTime time.Time,
	updateInfo *linkUpdateResourceInfo,
	deployCtx *DeployContext,
) (bool, error) {
	currentAttemptDuration := d.clock.Since(updateResourceBStartTime)
	linkName := linkInfo.element.LogicalName()
	accumDurationInfo := deployCtx.State.GetLinkDurationInfo(linkName)
	durations := determineLinkUpdateResourceBFinishedDurations(
		updateResourceBRetryInfo,
		currentAttemptDuration,
		accumDurationInfo,
	)
	deployCtx.State.SetLinkDurationInfo(linkName, durations)
	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkResourceBUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons:      updateInfo.failureReasons,
		CurrentStageAttempt: updateResourceBRetryInfo.attempt,
		UpdateTimestamp:     d.clock.Now().Unix(),
		Durations:           durations,
	}

	return true, nil
}

func (d *defaultLinkDeployer) createLinkUpdatingResourceBMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateResourceBRetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
) LinkDeployUpdateMessage {
	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdatingStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus: determinePreciseLinkUpdatingResourceBStatus(
			deployCtx.Rollback,
		),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateResourceBRetryInfo.attempt,
	}
}

func (d *defaultLinkDeployer) createLinkResourceBUpdatedMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateResourceBRetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
	updateResourceBStartTime time.Time,
) LinkDeployUpdateMessage {
	linkName := linkInfo.element.LogicalName()
	accumDurationInfo := deployCtx.State.GetLinkDurationInfo(linkName)
	durations := determineLinkUpdateResourceBFinishedDurations(
		updateResourceBRetryInfo,
		d.clock.Since(updateResourceBStartTime),
		accumDurationInfo,
	)
	deployCtx.State.SetLinkDurationInfo(linkName, durations)
	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		// We are still in the process of updating the link,
		// intermediary resources still need to be updated.
		Status: determineLinkUpdatingStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus:       determinePreciseLinkResourceBUpdatedStatus(deployCtx.Rollback),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateResourceBRetryInfo.attempt,
		Durations:           durations,
	}
}

func (d *defaultLinkDeployer) updateLinkIntermediaryResources(
	ctx context.Context,
	linkImplementation provider.Link,
	input *provider.LinkUpdateIntermediaryResourcesInput,
	linkInfo *deploymentElementInfo,
	updateIntermediariesRetryInfo *retryInfo,
	resourceAOutput *provider.LinkUpdateResourceOutput,
	resourceBOutput *provider.LinkUpdateResourceOutput,
	deployCtx *DeployContext,
) error {
	updateIntermediariesStartTime := d.clock.Now()
	deployCtx.Channels.LinkUpdateChan <- d.createLinkUpdatingIntermediaryResourcesMessage(
		linkInfo,
		deployCtx,
		updateIntermediariesRetryInfo,
		input.LinkUpdateType,
	)

	intermediaryResourcesOutput, err := linkImplementation.UpdateIntermediaryResources(ctx, input)
	if err != nil {
		if provider.IsRetryableError(err) {
			retryErr := err.(*provider.RetryableError)
			return d.handleUpdateLinkIntermediaryResourcesRetry(
				ctx,
				linkInfo,
				linkImplementation,
				updateIntermediariesRetryInfo,
				updateIntermediariesStartTime,
				&linkUpdateIntermediaryResourcesInfo{
					failureReasons: []string{retryErr.ChildError.Error()},
					input:          input,
				},
				resourceAOutput,
				resourceBOutput,
				deployCtx,
			)
		}

		if provider.IsLinkUpdateIntermediaryResourcesError(err) {
			linkUpdateIntermediariesError := err.(*provider.LinkUpdateIntermediaryResourcesError)
			return d.handleUpdateIntermediaryResourcesTerminalFailure(
				linkInfo,
				updateIntermediariesRetryInfo,
				updateIntermediariesStartTime,
				&linkUpdateIntermediaryResourcesInfo{
					failureReasons: linkUpdateIntermediariesError.FailureReasons,
					input:          input,
				},
				deployCtx,
			)
		}

		// For errors that are not wrapped in a provider error, the error is assumed to be fatal
		// and the deployment process will be stopped without reporting a failure state.
		// It is really important that adequate guidance is provided for provider developers
		// to ensure that all errors are wrapped in the appropriate provider error.
		return err
	}

	// We need to store the link deploy result before sending the status update
	// to ensure consistency in the temporary state of the link.
	// This makes sure that the link deploy result is available in the ephemeral state
	// when the status update handler persists the results to the state container.
	result := createLinkDeployResult(
		resourceAOutput,
		resourceBOutput,
		intermediaryResourcesOutput,
	)
	deployCtx.State.SetLinkDeployResult(linkInfo.element.LogicalName(), result)

	deployCtx.Channels.LinkUpdateChan <- d.createLinkIntermediariesUpdatedMessage(
		linkInfo,
		deployCtx,
		updateIntermediariesRetryInfo,
		input.LinkUpdateType,
		updateIntermediariesStartTime,
	)

	return nil
}

func (d *defaultLinkDeployer) createLinkIntermediariesUpdatedMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateIntermediariesRetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
	updateIntermediariesStartTime time.Time,
) LinkDeployUpdateMessage {
	linkName := linkInfo.element.LogicalName()
	accumDurationInfo := deployCtx.State.GetLinkDurationInfo(linkName)
	durations := determineLinkUpdateIntermediariesFinishedDurations(
		updateIntermediariesRetryInfo,
		d.clock.Since(updateIntermediariesStartTime),
		accumDurationInfo,
	)
	deployCtx.State.SetLinkDurationInfo(linkName, durations)

	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		// Updating intermediary resources is the last step in the link update process.
		Status: determineLinkOperationSuccessfullyFinishedStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus: determinePreciseLinkIntermediariesUpdatedStatus(
			deployCtx.Rollback,
		),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateIntermediariesRetryInfo.attempt,
		Durations:           durations,
	}
}

func (d *defaultLinkDeployer) handleUpdateLinkIntermediaryResourcesRetry(
	ctx context.Context,
	linkInfo *deploymentElementInfo,
	linkImplementation provider.Link,
	updateIntermediaryResourcesRetryInfo *retryInfo,
	updateIntermediaryResourcesStartTime time.Time,
	updateInfo *linkUpdateIntermediaryResourcesInfo,
	resourceAOutput *provider.LinkUpdateResourceOutput,
	resourceBOutput *provider.LinkUpdateResourceOutput,
	deployCtx *DeployContext,
) error {
	currentAttemptDuration := d.clock.Since(updateIntermediaryResourcesStartTime)
	nextRetryInfo := addRetryAttempt(
		updateIntermediaryResourcesRetryInfo,
		currentAttemptDuration,
	)
	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkIntermediariesUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons: updateInfo.failureReasons,
		// Attempt and retry information included the status update is specific to
		// updating intermediary resources, each component of a link change will have its own
		// number of attempts and retry information.
		CurrentStageAttempt:  updateIntermediaryResourcesRetryInfo.attempt,
		CanRetryCurrentStage: !nextRetryInfo.exceededMaxRetries,
		UpdateTimestamp:      d.clock.Now().Unix(),
		// Attempt durations will be accumulated and sent in the status updates
		// for each subsequent retry.
		// Total duration will be calculated if retry limit is exceeded.
		Durations: determineLinkUpdateIntermediariesRetryFailureDurations(
			nextRetryInfo,
		),
	}

	if !nextRetryInfo.exceededMaxRetries {
		waitTimeMS := provider.CalculateRetryWaitTimeMS(nextRetryInfo.policy, nextRetryInfo.attempt)
		time.Sleep(time.Duration(waitTimeMS) * time.Millisecond)
		return d.updateLinkIntermediaryResources(
			ctx,
			linkImplementation,
			updateInfo.input,
			linkInfo,
			nextRetryInfo,
			resourceAOutput,
			resourceBOutput,
			deployCtx,
		)
	}

	return nil
}

func (d *defaultLinkDeployer) handleUpdateIntermediaryResourcesTerminalFailure(
	linkInfo *deploymentElementInfo,
	updateIntermediariesRetryInfo *retryInfo,
	updateIntermediariesStartTime time.Time,
	updateInfo *linkUpdateIntermediaryResourcesInfo,
	deployCtx *DeployContext,
) error {
	currentAttemptDuration := d.clock.Since(updateIntermediariesStartTime)
	linkName := linkInfo.element.LogicalName()
	accumDurationInfo := deployCtx.State.GetLinkDurationInfo(linkName)
	durations := determineLinkUpdateIntermediariesFinishedDurations(
		updateIntermediariesRetryInfo,
		currentAttemptDuration,
		accumDurationInfo,
	)
	deployCtx.State.SetLinkDurationInfo(linkName, durations)

	deployCtx.Channels.LinkUpdateChan <- LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdateFailedStatus(
			deployCtx.Rollback,
			updateInfo.input.LinkUpdateType,
		),
		PreciseStatus: determinePreciseLinkIntermediariesUpdateFailedStatus(
			deployCtx.Rollback,
		),
		FailureReasons:      updateInfo.failureReasons,
		CurrentStageAttempt: updateIntermediariesRetryInfo.attempt,
		UpdateTimestamp:     d.clock.Now().Unix(),
		Durations:           durations,
	}

	return nil
}

func (d *defaultLinkDeployer) createLinkUpdatingIntermediaryResourcesMessage(
	linkInfo *deploymentElementInfo,
	deployCtx *DeployContext,
	updateIntermediariesRetryInfo *retryInfo,
	linkUpdateType provider.LinkUpdateType,
) LinkDeployUpdateMessage {
	return LinkDeployUpdateMessage{
		InstanceID: linkInfo.instanceID,
		LinkID:     linkInfo.element.ID(),
		LinkName:   linkInfo.element.LogicalName(),
		Status: determineLinkUpdatingStatus(
			deployCtx.Rollback,
			linkUpdateType,
		),
		PreciseStatus: determinePreciseLinkUpdatingIntermediariesStatus(
			deployCtx.Rollback,
		),
		UpdateTimestamp:     d.clock.Now().Unix(),
		CurrentStageAttempt: updateIntermediariesRetryInfo.attempt,
	}
}

func getResourceInfoFromStateForLinkDeployment(
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

func createLinkDeployResult(
	resourceAOutput *provider.LinkUpdateResourceOutput,
	resourceBOutput *provider.LinkUpdateResourceOutput,
	intermediaryResourcesOutput *provider.LinkUpdateIntermediaryResourcesOutput,
) *LinkDeployResult {
	resourceAOutputLinkData := getResourceOutputLinkData(resourceAOutput)
	resourceBOutputLinkData := getResourceOutputLinkData(resourceBOutput)
	intermediaryResourcesOutputLinkData := getIntermediaryResourcesOutputLinkData(
		intermediaryResourcesOutput,
	)
	intermediaryResourceStates := getIntermediaryResourcesOutputStates(
		intermediaryResourcesOutput,
	)

	return &LinkDeployResult{
		IntermediaryResourceStates: intermediaryResourceStates,
		LinkData: core.MergeMaps(
			resourceAOutputLinkData,
			resourceBOutputLinkData,
			intermediaryResourcesOutputLinkData,
		),
	}
}

func getResourceOutputLinkData(output *provider.LinkUpdateResourceOutput) *core.MappingNode {
	if output == nil {
		return nil
	}

	return output.LinkData
}

func getIntermediaryResourcesOutputLinkData(
	output *provider.LinkUpdateIntermediaryResourcesOutput,
) *core.MappingNode {
	if output == nil {
		return nil
	}

	return output.LinkData
}

func getIntermediaryResourcesOutputStates(
	output *provider.LinkUpdateIntermediaryResourcesOutput,
) []*state.LinkIntermediaryResourceState {
	if output == nil {
		return nil
	}

	return output.IntermediaryResourceStates
}
