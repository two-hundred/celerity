package container

import (
	"context"
	"time"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/subengine"
)

const (
	prepareFailureMessage = "failed to load instance state while preparing to deploy"
)

func (c *defaultBlueprintContainer) Deploy(
	ctx context.Context,
	input *DeployInput,
	channels *DeployChannels,
	paramOverrides core.BlueprintParams,
) {

	state := &deploymentState{
		pendingLinks:               map[string]*linkPendingCompletion{},
		resourceNamePendingLinkMap: map[string][]string{},
		previousResourceState:      map[string]*state.ResourceState{},
		previousChildState:         map[string]*state.InstanceState{},
		previousLinkState:          map[string]*state.LinkState{},
	}

	if input.Changes == nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			core.InstanceStatusDeployFailed,
			[]string{"an empty set of changes was provided for deployment"},
			0,
		)
		return
	}

	startTime := c.clock.Now()
	channels.DeploymentUpdateChan <- DeploymentUpdateMessage{
		InstanceID: input.InstanceID,
		Status:     core.InstanceStatusPreparing,
	}

	// Use the same behaviour as change staging to extract the nodes
	// that need to be deployed or updated where they are grouped for concurrent deployment
	// and in order based on links, references and use of the `dependsOn` property.
	processed, err := c.processBlueprint(
		ctx,
		subengine.ResolveForDeployment,
		input.Changes,
		paramOverrides,
	)
	if err != nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			core.InstanceStatusDeployFailed,
			[]string{prepareFailureMessage},
			c.clock.Since(startTime),
		)
		return
	}

	flattenedNodes := core.Flatten(processed.parallelGroups)

	instances := c.stateContainer.Instances()
	currentInstanceState, err := instances.Get(ctx, input.InstanceID)
	if err != nil {
		channels.FinishChan <- c.createDeploymentFinishedMessage(
			input.InstanceID,
			core.InstanceStatusDeployFailed,
			[]string{prepareFailureMessage},
			c.clock.Since(startTime),
		)
		return
	}

	_, err = c.removeElements(
		ctx,
		input,
		&deployContext{
			startTime:             startTime,
			state:                 state,
			rollback:              input.Rollback,
			channels:              channels,
			paramOverrides:        paramOverrides,
			instanceStateSnapshot: &currentInstanceState,
			resourceProviders:     processed.resourceProviderMap,
		},
		flattenedNodes,
	)
	if err != nil {
		channels.ErrChan <- err
		return
	}

	// Get all components to be deployed or updated.
	// Order components based on dependencies in the blueprint.
	// Group components so those that are not connected can be deployed in parallel.
	// Unlike with change staging, groups are not executed as a unit, they are used as
	// pools to look for components that can be deployed based on the current state of deployment.
	// For each component to be created or updated (including recreated children):
	//  - If resource, resolve condition, check if condition is met, if not, skip deployment.
	//  - call Deploy method component (resource or child blueprint)
	//      - handle specialised provider errors (retryable, resource deploy errors etc.)
	//  - If component is resource and is in config complete status, check if any of its dependents
	//    require the resource to be stable before they can be deployed.
	//    - If so, wait for the resource to be stable before deploying the dependent.
	//    - If not, begin deploying the dependent.
	//  - Check if there are any links that can be deployed based on the current state of deployment.
	//     - If so, deploy the link.
	//	- On failure that can not be retried, roll back all changes successfully made for the current deployment.
	//     - See notes on deployment rollback for how this should be implemented for different states.
}

func (c *defaultBlueprintContainer) createDeploymentFinishedMessage(
	instanceID string,
	status core.InstanceStatus,
	failureReasons []string,
	elapsedTime time.Duration,
) DeploymentFinishedMessage {
	elapsedMilliseconds := core.FractionalMilliseconds(elapsedTime)
	return DeploymentFinishedMessage{
		InstanceID:      instanceID,
		Status:          status,
		FailureReasons:  failureReasons,
		FinishTimestamp: c.clock.Now().Unix(),
		Durations: &state.InstanceCompletionDuration{
			TotalDuration: &elapsedMilliseconds,
		},
	}
}

// Keeps track of state regarding when links are ready to be processed
// along with the previous state of a blueprint element to allow for rolling back.
// All instance state including statuses of resources, links and child blueprints
// are stored in the state container.
// This is a temporary representation of the state of the deployment
// that is not persisted.
// The state container does not support revisions/history of the state of a blueprint instance,
// so the previous state is held in memory during deployment by the blueprint container.
// Services built on top of the blueprint framework will often provide revisions/history
// for the state of a blueprint instance.
type deploymentState struct {
	// A mapping of a logical link name to the pending link completion state for links
	// that need to be deployed or updated.
	// A link ID in this context is made up of the resource names of the two resources
	// that are linked together.
	// For example, if resource A is linked to resource B, the link name would be "A::B".
	// This is used to keep track of when links are ready to be deployed or updated.
	// This does not hold the state of the link, only information about when the link is ready
	// to be deployed or updated.
	// Link removals are not tracked here as they do not depend on resource state changes,
	// removal of links happens before resources in the link relationship are processed.
	pendingLinks map[string]*linkPendingCompletion
	// A mapping of resource names to pending links that include the resource.
	resourceNamePendingLinkMap map[string][]string
	// A mapping of logical resource names to the previous state of the resource.
	// An entry with a ResourceID of "" indicates that the resource was not previously deployed.
	previousResourceState map[string]*state.ResourceState
	// A mapping of logical child blueprint names to the previous state of the child blueprint.
	// An entry with a InstanceID of "" indicates that the child blueprint was not previously deployed.
	previousChildState map[string]*state.InstanceState
	// A mapping of logical link names ({resourceA}::{resourceB}) to the previous state of the link.
	// An entry with a LinkID of "" indicates that the link was not previously deployed.
	previousLinkState map[string]*state.LinkState
	// Mutex is required as resources can be deployed concurrently.
	// mu sync.Mutex
}

type deployContext struct {
	startTime time.Time
	rollback  bool
	state     *deploymentState
	channels  *DeployChannels
	// A snapshot of the instance state taken before deployment.
	instanceStateSnapshot *state.InstanceState
	paramOverrides        core.BlueprintParams
	resourceProviders     map[string]provider.Provider
	currentGroupIndex     int
}

func deployContextWithChannels(
	deployCtx *deployContext,
	channels *DeployChannels,
) *deployContext {
	return &deployContext{
		startTime:         deployCtx.startTime,
		state:             deployCtx.state,
		channels:          channels,
		paramOverrides:    deployCtx.paramOverrides,
		resourceProviders: deployCtx.resourceProviders,
	}
}

func deployContextWithGroup(
	deployCtx *deployContext,
	groupIndex int,
) *deployContext {
	return &deployContext{
		startTime:         deployCtx.startTime,
		state:             deployCtx.state,
		channels:          deployCtx.channels,
		paramOverrides:    deployCtx.paramOverrides,
		resourceProviders: deployCtx.resourceProviders,
		currentGroupIndex: groupIndex,
	}
}

type deployUpdateMessageWrapper struct {
	// resourceUpdateMessage *ResourceDeployUpdateMessage
	// linkUpdateMessage     *LinkDeployUpdateMessage
	// childUpdateMessage    *ChildDeployUpdateMessage
}

type retryInfo struct {
	attempt            int
	exceededMaxRetries bool
	policy             *provider.RetryPolicy
	attemptDurations   []float64
}

type deploymentElementInfo struct {
	element    state.Element
	instanceID string
}

// DeployChannels contains all the channels required to stream
// deployment events.
type DeployChannels struct {
	// ResourceUpdateChan receives messages about the status of deployment for resources.
	ResourceUpdateChan chan ResourceDeployUpdateMessage
	// LinkUpdateChan receives messages about the status of deployment for links.
	LinkUpdateChan chan LinkDeployUpdateMessage
	// ChildUpdateChan receives messages about the status of deployment for child blueprints.
	ChildUpdateChan chan ChildDeployUpdateMessage
	// DeploymentUpdateChan receives messages about the status of the blueprint instance deployment.
	DeploymentUpdateChan chan DeploymentUpdateMessage
	// FinishChan is used to signal that the blueprint instance deployment has finished,
	// the message will contain the final status of the deployment.
	FinishChan chan DeploymentFinishedMessage
	// ErrChan is used to signal that an unexpected error occurred during deployment of changes.
	ErrChan chan error
}

// ResourceDeployUpdateMessage provides a message containing status updates
// for resources being deployed.
// Deployment messages report on status changes for resources,
// the state of a resource will need to be fetched from the state container
// to get further information about the state of the resource.
type ResourceDeployUpdateMessage struct {
	// InstanceID is the ID of the blueprint instance
	// the message is associated with.
	// As updates are sent for parent and child blueprints,
	// this ID is used to differentiate between them.
	InstanceID string `json:"instanceId"`
	// ResourceID is the globally unique ID of the resource.
	ResourceID string `json:"resourceId"`
	// ResourceName is the logical name of the resource
	// as defined in the source blueprint.
	ResourceName string `json:"resourceName"`
	// Group is the group number the resource belongs to relative to the ordering
	// for components in the current blueprint associated with the instance ID.
	// A group is a collection of items that can be deployed at the same time.
	Group int `json:"group"`
	// Status holds the high-level status of the resource.
	Status core.ResourceStatus `json:"status"`
	// PreciseStatus holds the detailed status of the resource.
	PreciseStatus core.PreciseResourceStatus `json:"preciseStatus"`
	// FailureReasons holds a list of reasons why the resource failed to deploy
	// if the status update is for a failure.
	FailureReasons []string `json:"failureReasons,omitempty"`
	// Attempt is the current attempt number for deploying or destroying the resource.
	Attempt int `json:"attempt"`
	// CanRetry indicates if the operation for the resource can be retried
	// after this attempt.
	CanRetry bool `json:"canRetry"`
	// UpdateTimestamp is the unix timestamp in seconds for
	// when the status update occurred.
	UpdateTimestamp int64 `json:"updateTimestamp"`
	// Durations holds duration information for a resource deployment.
	// Duration information is attached on one of the following precise status updates:
	// - PreciseResourceStatusConfigComplete
	// - PreciseResourceStatusCreated
	// - PreciseResourceStatusCreateFailed
	// - PreciseResourceStatusCreateRollbackFailed
	// - PreciseResourceStatusCreateRollbackComplete
	// - PreciseResourceStatusDestroyed
	// - PreciseResourceStatusDestroyFailed
	// - PreciseResourceStatusDestroyRollbackFailed
	// - PreciseResourceStatusDestroyRollbackComplete
	// - PreciseResourceStatusUpdateConfigComplete
	// - PreciseResourceStatusUpdated
	// - PreciseResourceStatusUpdateFailed
	// - PreciseResourceStatusUpdateRollbackFailed
	// - PreciseResourceStatusUpdateRollbackComplete
	Durations *state.ResourceCompletionDurations `json:"durations,omitempty"`
}

// ResourceChangesMessage provides a message containing status updates
// for resources being deployed.
// Deployment messages report on status changes for resources,
// the state of a resource will need to be fetched from the state container
// to get further information about the state of the resource.
type LinkDeployUpdateMessage struct {
	// InstanceID is the ID of the blueprint instance
	// the message is associated with.
	// As updates are sent for parent and child blueprints,
	// this ID is used to differentiate between them.
	InstanceID string `json:"instanceId"`
	// LinkID is the globally unique ID of the link.
	LinkID string `json:"linkId"`
	// LinkName is the logic name of the link in the blueprint.
	// This is a combination of the 2 resources that are linked.
	// For example, if a link is between a VPC and a subnet,
	// the link name would be "vpc::subnet".
	LinkName string `json:"linkName"`
	// Status holds the high-level status of the link.
	Status core.LinkStatus `json:"status"`
	// PreciseStatus holds the detailed status of the link.
	PreciseStatus core.PreciseLinkStatus `json:"preciseStatus"`
	// FailureReasons holds a list of reasons why the link failed to deploy
	// if the status update is for a failure.
	FailureReasons []string `json:"failureReasons,omitempty"`
	// Attempt is the current attempt number for deploying the link.
	Attempt int `json:"attempt"`
	// UpdateTimestamp is the unix timestamp in seconds for
	// when the status update occurred.
	UpdateTimestamp int64 `json:"updateTimestamp"`
	// Durations holds duration information for a link deployment.
	// Duration information is attached on one of the following precise status updates:
	// - PreciseLinkStatusResourceAUpdated
	// - PreciseLinkStatusResourceAUpdateFailed
	// - PreciseLinkStatusResourceBUpdated
	// - PreciseLinkStatusResourceBUpdateFailed
	// - PreciseLinkStatusIntermediaryResourcesUpdated
	// - PreciseLinkStatusIntermediaryResourceUpdateFailed
	// - PreciseLinkStatusComplete
	Durations *state.LinkCompletionDurations `json:"durations,omitempty"`
}

// ChildDeployUpdateMessage provides a message containing status updates
// for child blueprints being deployed.
// Deployment messages report on status changes for child blueprints,
// the state of a child blueprint will need to be fetched from the state container
// to get further information about the state of the child blueprint.
type ChildDeployUpdateMessage struct {
	// ParentInstanceID is the ID of the parent blueprint instance
	// the message is associated with.
	ParentInstanceID string `json:"parentInstanceId"`
	// ChildInstanceID is the ID of the child blueprint instance
	// the message is associated with.
	ChildInstanceID string `json:"instanceId"`
	// ChildName is the logical name of the child blueprint
	// as defined in the source blueprint as an include.
	ChildName string `json:"childName"`
	// Group is the group number the child blueprint belongs to relative to the ordering
	// for components in the current blueprint associated with the parent instance ID.
	Group int `json:"group"`
	// Status holds the status of the child blueprint.
	Status core.InstanceStatus `json:"status"`
	// FailureReasons holds a list of reasons why the child blueprint failed to deploy
	// if the status update is for a failure.
	FailureReasons []string `json:"failureReasons,omitempty"`
	// UpdateTimestamp is the unix timestamp in seconds for
	// when the status update occurred.
	UpdateTimestamp int64 `json:"updateTimestamp"`
	// Durations holds duration information for a child blueprint deployment.
	// Duration information is attached on one of the following status updates:
	// - InstanceStatusDeployed
	// - InstanceStatusDeployFailed
	// - InstanceStatusDestroyed
	// - InstanceStatusUpdated
	// - InstanceStatusUpdateFailed
	Durations *state.InstanceCompletionDuration `json:"durations,omitempty"`
}

// DeploymentUpdateMessage provides a message containing a blueprint-wide status update
// for the deployment of a blueprint instance.
type DeploymentUpdateMessage struct {
	// InstanceID is the ID of the blueprint instance
	// the message is associated with.
	InstanceID string `json:"instanceId"`
	// Status holds the status of the instance deployment.
	Status core.InstanceStatus `json:"status"`
}

// DeploymentFinishedMessage provides a message containing the final status
// of the blueprint instance deployment.
type DeploymentFinishedMessage struct {
	// InstanceID is the ID of the blueprint instance
	// the message is associated with.
	InstanceID string `json:"instanceId"`
	// Status holds the status of the instance deployment.
	Status core.InstanceStatus `json:"status"`
	// FailureReasons holds a list of reasons why the instance failed to deploy
	// if the final status is a failure.
	FailureReasons []string `json:"failureReasons,omitempty"`
	// FinishTimestamp is the unix timestamp in seconds for
	// when the deployment finished.
	FinishTimestamp int64 `json:"finishTimestamp"`
	// Durations holds duration information for the blueprint deployment.
	// Duration information is attached on one of the following status updates:
	// - InstanceStatusDeploying (preparation phase duration only)
	// - InstanceStatusDeployed
	// - InstanceStatusDeployFailed
	// - InstanceStatusDestroyed
	// - InstanceStatusUpdated
	// - InstanceStatusUpdateFailed
	Durations *state.InstanceCompletionDuration `json:"durations,omitempty"`
}
