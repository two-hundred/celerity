package provider

import (
	"context"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type Link interface {
	// StageChanges must detail the changes that will be made when a deployment of the loaded blueprint
	// for the link between two resources and blueprint instance provided in resourceInfo.
	StageChanges(
		ctx context.Context,
		input *LinkStageChangesInput,
	) (*LinkStageChangesOutput, error)
	// Deploy deals with deploying a link between two resources in the upstream provider.
	// The behaviour of deploy is completely down to the implementation of a link provider and how long
	// a link is likely to take to deploy. The state will be synchronised periodically and will reflect the current
	// state for long running deployments that we won't be waiting around for.
	// Parameters are passed into Deploy for extra context, blueprint variables will have already
	// been substituted at this stage and must be used instead of the passed in params argument
	// to ensure consistency between the staged changes that are reviewed and the deployment itself.
	Deploy(ctx context.Context, input *LinkDeployInput) (*LinkDeployOutput, error)
	// GetPriorityResourceType retrieves the resource type in the relationship
	// that must be deployed first. This will be empty for links where one resource type does not
	// need to be deployed before the other.
	GetPriorityResourceType(ctx context.Context, input *LinkGetPriorityResourceTypeInput) (*LinkGetPriorityResourceTypeOutput, error)
	// GetType deals with retrieving the type of the link in relation to the two resource
	// types it provides a relationship between.
	GetType(ctx context.Context, input *LinkGetTypeInput) (*LinkGetTypeOutput, error)
	// GetKind tells us whether the link is "hard" or "soft" link.
	// A hard link is where the priority resource type must be created first.
	// A soft link is where it does not matter which resource type in the relationship
	// is created first.
	GetKind(ctx context.Context, input *LinkGetKindInput) (*LinkGetKindOutput, error)
	// HandleResourceTypeAError deals with handling errors in
	// the deployment of the first of the two linked resources.
	HandleResourceTypeAError(ctx context.Context, input *LinkHandleResourceTypeErrorInput) error
	// HandleResourceTypeBError deals with handling errors
	// in the second of the two linked resources.
	HandleResourceTypeBError(ctx context.Context, input *LinkHandleResourceTypeErrorInput) error
}

// LinkStageChangesInput provides the input required to
// stage changes for a link between two resources.
type LinkStageChangesInput struct {
	ResourceAInfo *ResourceInfo
	ResourceBInfo *ResourceInfo
	Params        core.BlueprintParams
}

// LinkStageChangesOutput provides the output from staging changes
// for a link between two resources.
type LinkStageChangesOutput struct {
	Changes *LinkChanges
}

// LinkDeployInput provides the input required to
// deploy a link between two resources.
type LinkDeployInput struct {
	Changes       *LinkChanges
	ResourceAInfo *ResourceInfo
	ResourceBInfo *ResourceInfo
	Params        core.BlueprintParams
}

// LinkDeployOutput provides the output from deploying
// a link between two resources.
type LinkDeployOutput struct {
	ResourceAState *state.ResourceState
	ResourceBState *state.ResourceState
	LinkState      *state.LinkState
}

// LinkPriorityResourceTypeOutput provides the input for retrieving
// the priority resource type in a link relationship.
type LinkGetPriorityResourceTypeInput struct {
	Params core.BlueprintParams
}

// LinkPriorityResourceTypeOutput provides the output for retrieving
// the priority resource type in a link relationship.
type LinkGetPriorityResourceTypeOutput struct {
	PriorityResourceType string
}

// LinkGetKindInput provides the input for retrieving the kind of link.
type LinkGetKindInput struct {
	Params core.BlueprintParams
}

// LinkGetKindOutput provides the output for retrieving the kind of link.
type LinkGetKindOutput struct {
	Kind LinkKind
}

// LinkGetTypeOutput provides the output for retrieving the type of link
// with respect to the two resource types it provides a relationship between.
type LinkGetTypeInput struct {
	Params core.BlueprintParams
}

// LinkGetTypeOutput provides the output for retrieving the type of link
// with respect to the two resource types it provides a relationship between.
type LinkGetTypeOutput struct {
	Type string
}

// HandleResourceTypeErrorInput provides the input for handling errors
// related to the deployment of a resource type in a link relationship.
type LinkHandleResourceTypeErrorInput struct {
	ResourceInfo *ResourceInfo
	Params       core.BlueprintParams
}

// LinkKind provides a way to categorise links to help determine the order
// in which resources need to be deployed when a blueprint instance is being deployed.
type LinkKind string

const (
	// LinkKindHard is the type of link where the priority resource type
	// must be created before the other resource type in the relationship.
	LinkKindHard LinkKind = "hard"
	// LinkKindSoft is the type of link where it does not matter
	// which of the two resource types in the relationship is created
	// first.
	LinkKindSoft LinkKind = "soft"
)

// Changes provides a set of modified fields along with a version
// of the resource schema (includes metadata labels and annotations) and spec
// that has already had all it's variables substituted.
type LinkChanges struct {
	ResourceTypeAModifiedFields  []string
	ResourceTypeANewFields       []string
	ResourceTypeARemovedFields   []string
	ResourceTypeAUnchangedFields []string
	ResourceTypeBModifiedFields  []string
	ResourceTypeBNewFields       []string
	ResourceTypeBRemovedFields   []string
	ResourceTypeBUnchangedFields []string
	IntermediaryResourceChanges  []LinkIntermediaryResourceChanges
}

// LinkIntermediaryResourceChanges provides a set of modified fields
// for an intermediary resource in a link relationship.
type LinkIntermediaryResourceChanges struct {
	ResourceType    string
	ModifiedFields  []string
	NewFields       []string
	RemovedFields   []string
	UnchangedFields []string
}
