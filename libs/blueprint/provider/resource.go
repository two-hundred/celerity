package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

// Resource provides the interface for a resource
// that a provider can contain which includes logic for validating,
// transforming, linking and deploying a resource.
type Resource interface {
	// CustomValidate provides support for custom validation that goes beyond
	// the spec schema validation provided by the resource's spec definition.
	CustomValidate(ctx context.Context, input *ResourceValidateInput) (*ResourceValidateOutput, error)
	// GetSpecDefinition retrieves the spec definition for a resource,
	// this is the first line of validation for a resource in a blueprint and is also
	// useful for validating references to a resource instance
	// in a blueprint and for providing definitions for docs and tooling.
	// The spec defines both the schema for the resource spec fields that can be defined
	// by users in a blueprint and computed fields that are derived from the deployed
	// resource in the external provider (e.g. Lambda ARN in AWS).
	GetSpecDefinition(ctx context.Context, input *ResourceGetSpecDefinitionInput) (*ResourceGetSpecDefinitionOutput, error)
	// CanLinkTo specifices the list of resource types the current resource type
	// can link to.
	CanLinkTo(ctx context.Context, input *ResourceCanLinkToInput) (*ResourceCanLinkToOutput, error)
	// StabilisedDependencies retrieves a list of resource types that must be stabilised
	// before the current resource can be deployed when another resource of one of the specified types
	// is a dependency of the current resource in a blueprint.
	StabilisedDependencies(ctx context.Context, input *ResourceStabilisedDependenciesInput) (*ResourceStabilisedDependenciesOutput, error)
	// IsCommonTerminal specifies whether this resource is expected to have a common use-case
	// as a terminal resource that does not link out to other resources.
	// This is useful for providing useful warnings to users about their blueprints
	// without overloading them with warnings for all resources that don't have any outbound
	// links that could have.
	IsCommonTerminal(ctx context.Context, input *ResourceIsCommonTerminalInput) (*ResourceIsCommonTerminalOutput, error)
	// GetType deals with retrieving the namespaced type for a resource in a blueprint spec.
	GetType(ctx context.Context, input *ResourceGetTypeInput) (*ResourceGetTypeOutput, error)
	// GetTypeDescription deals with retrieving the description for a resource type in a blueprint spec
	// that can be used for documentation and tooling.
	// Markdown and plain text formats are supported.
	GetTypeDescription(ctx context.Context, input *ResourceGetTypeDescriptionInput) (*ResourceGetTypeDescriptionOutput, error)
	// Deploy deals with deploying a resource with the upstream resource provider.
	// The behaviour of deploy is to create or update the resource configuration and return the resource
	// spec state once the configuration has been created or updated.
	// Deploy should not wait for the resource to be in a stable state before returning,
	// the framework will call the HasStabilised method periodically when waiting for a resource
	// to stabilise.
	// Parameters are passed into Deploy for extra context, blueprint variables will have already
	// been substituted at this stage and must be used instead of the passed in params argument
	// to ensure consistency between the staged changes that are reviewed and the deployment itself.
	Deploy(ctx context.Context, input *ResourceDeployInput) (*ResourceDeployOutput, error)
	// HasStabilised deals with checking if a resource has stabilised after being deployed.
	// This is important for resources that require a stable state before other resources can be deployed.
	// This is only used when creating or updating a resource, not when destroying a resource.
	HasStabilised(ctx context.Context, input *ResourceHasStabilisedInput) (*ResourceHasStabilisedOutput, error)
	// GetExternalState deals with getting a the state of the resource from the resource provider.
	// (e.g. AWS or Google Cloud)
	// The blueprint instance and resource should be
	// attached to the resource in the external provider
	// in order to fetch its status and sync up.
	GetExternalState(ctx context.Context, input *ResourceGetExternalStateInput) (*ResourceGetExternalStateOutput, error)
	// Destroy deals with destroying a resource instance if its current
	// state is successfully deployed or cleaning up a corrupt or partially deployed
	// resource instance.
	// The resource instance should be completely removed from the external provider
	// as a result of this operation; this is essential for when
	// another element to be removed from a blueprint
	// requires a resource to be completely removed from the external provider.
	// There is no "config complete" equivalent for destroying a resource and
	// "HasStabilised" is designed to be used for resources being created or
	// updated.
	Destroy(ctx context.Context, input *ResourceDestroyInput) error
}

// ResourceInfo provides all the information needed for a resource
// including the blueprint schema data with annotations, labels
// and the spec as a core mapping node.
type ResourceInfo struct {
	// ResourceID holds the ID of a resource when in the context
	// of a blueprint instance when deploying or staging changes.
	// Sometimes staging changes is independent of an instance and is used to compare
	// two vesions of a blueprint in which
	// case the resource ID will be empty.
	ResourceID string `json:"resourceId"`
	// ResourceName holds the name of the resource in the blueprint spec.
	// This is useful for new resources that do not have any current resource state.
	ResourceName string `json:"resourceName"`
	// InstanceID holds the ID of the blueprint instance
	// that the current resource belongs to.
	// This could be empty if the resource is being staged
	// for an initial deployment.
	InstanceID string `json:"instanceId"`
	// CurrentResourceState holds the current state of the resource
	// for which changes are being staged.
	// The ResourceInfo struct is passed into resource implementation plugins
	// for deployment.
	// A resource implementation should be guarded from the details of where
	// the state is stored and how it is retrieved,
	// the state should be provided to resource plugins by the blueprint
	// engine.
	// If this is a nil pointer, it means that the resource is new and does not have
	// a current state.
	CurrentResourceState *state.ResourceState `json:"currentResourceState"`
	// ResourceWithResolvedSubs holds a version of a resource for which all ${..}
	// substitutions have been applied.
	ResourceWithResolvedSubs *ResolvedResource `json:"resourceWithResolvedSubs"`
}

// ResolvedResource provides a version of a resource for which all ${..}
// substitutions have been applied.
// Mapping nodes replace StringOrSubstitutions from the blueprint schema representation
// of the resource.
type ResolvedResource struct {
	Type         *schema.ResourceTypeWrapper `json:"type"`
	Description  *core.MappingNode           `json:"description,omitempty"`
	Metadata     *ResolvedResourceMetadata   `json:"metadata,omitempty"`
	Condition    *ResolvedResourceCondition  `json:"condition,omitempty"`
	LinkSelector *schema.LinkSelector        `json:"linkSelector,omitempty"`
	Spec         *core.MappingNode           `json:"spec"`
}

// ResolvedResourceMetadata provides a resolved version of the metadata
// for a resource where all substitutions have been applied.
type ResolvedResourceMetadata struct {
	DisplayName *core.MappingNode `json:",omitempty"`
	Annotations *core.MappingNode `json:"annotations,omitempty"`
	Labels      *schema.StringMap `json:"labels,omitempty"`
	Custom      *core.MappingNode `json:"custom,omitempty"`
}

// ResolvedResourceCondition provides a resolved version of the condition
// for a resource where all substitutions have been applied.
type ResolvedResourceCondition struct {
	// A list of conditions that must all be true.
	And []*ResolvedResourceCondition `json:"and,omitempty"`
	// A list of conditions where at least one must be true.
	Or []*ResolvedResourceCondition `json:"or,omitempty"`
	// A condition that will be negated.
	Not *ResolvedResourceCondition `json:"not,omitempty"`
	// A condition expression that is expected
	// to be a substitution that resolves to a boolean.
	StringValue *core.MappingNode `json:"-"`
}

func (c *ResolvedResourceCondition) UnmarshalJSON(data []byte) error {
	if strings.HasPrefix(string(data), "\"") {
		stringVal := &core.MappingNode{}
		if err := json.Unmarshal(data, &stringVal); err == nil {
			c.StringValue = stringVal
			return nil
		} else {
			return err
		}
	}

	type conditionAlias ResolvedResourceCondition
	var alias conditionAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	c.And = alias.And
	c.Or = alias.Or
	c.Not = alias.Not

	if (len(c.And) > 0 && len(c.Or) > 0) ||
		(len(c.Or) > 0 && c.Not != nil) ||
		(len(c.And) > 0 && c.Not != nil) {
		return fmt.Errorf(
			"an invalid resource condition has been provided, only one of \"and\", \"or\" or \"not\" can be set",
		)
	}

	return nil
}

func (c *ResolvedResourceCondition) MarshalJSON() ([]byte, error) {
	if c.StringValue != nil {
		return json.Marshal(c.StringValue)
	}

	type conditionAlias ResolvedResourceCondition
	var alias conditionAlias
	alias.And = c.And
	alias.Or = c.Or
	alias.Not = c.Not
	return json.Marshal(alias)
}

// ResourceValidateParams provides the input data needed for a resource to
// be validated.
type ResourceValidateInput struct {
	SchemaResource *schema.Resource
	Params         core.BlueprintParams
}

// ResourceValidateOutput provides the output data from validating a resource
// which includes a list of diagnostics that detail issues with the resource.
type ResourceValidateOutput struct {
	Diagnostics []*core.Diagnostic
}

// ResourceGetSpecDefinitionInput provides the input data needed for a resource to
// provide a spec definition.
type ResourceGetSpecDefinitionInput struct {
	Params core.BlueprintParams
}

// ResourceGetSpecDefinitionOutput provides the output data from providing a spec definition
// for a resource.
type ResourceGetSpecDefinitionOutput struct {
	SpecDefinition *ResourceSpecDefinition
}

// ResourceCanLinkToInput provides the input data needed for a resource to
// determine what types of resources it can link to.
type ResourceCanLinkToInput struct {
	Params core.BlueprintParams
}

// ResourceCanLinkToOutput provides the output data from determining what types of resources
// a given resource can link to.
type ResourceCanLinkToOutput struct {
	CanLinkTo []string
}

// ResourceStabilisedDependenciesInput provides the input data needed for a resource to
// determine what resource types must be stabilised before the current resource can be deployed.
type ResourceStabilisedDependenciesInput struct {
	Params core.BlueprintParams
}

// ResourceStabilisedDependenciesOutput provides the output data from determining what resource types
// must be stabilised before the current resource can be deployed.
type ResourceStabilisedDependenciesOutput struct {
	StabilisedDependencies []string
}

// ResourceIsCommonTerminalInput provides the input data needed for a resource to
// determine if it is a common terminal resource.
type ResourceIsCommonTerminalInput struct {
	Params core.BlueprintParams
}

// ResourceIsCommonTerminalOutput provides the output data from determining if a resource
// is a common terminal resource.
type ResourceIsCommonTerminalOutput struct {
	IsCommonTerminal bool
}

// ResourceDeployInput provides the input data needed for a resource to
// be deployed.
type ResourceDeployInput struct {
	Changes *Changes
	Params  core.BlueprintParams
}

// ResourceGetTypeInput provides the input data needed for a resource to
// determine the type of a resource in a blueprint spec.
type ResourceGetTypeInput struct {
	Params core.BlueprintParams
}

// ResourceGetTypeOutput provides the output data from determining the type of a resource
// in a blueprint spec.
type ResourceGetTypeOutput struct {
	Type string
}

// ResourceGetTypeDescriptionInput provides the input data needed for a resource to
// retrieve a description of the type of a resource in a blueprint spec.
type ResourceGetTypeDescriptionInput struct {
	Params core.BlueprintParams
}

// ResourceGetTypeDescriptionOutput provides the output data from retrieving a description
// of the type of a resource in a blueprint spec.
type ResourceGetTypeDescriptionOutput struct {
	MarkdownDescription  string
	PlainTextDescription string
}

// ResourceDeployOutput provides the output data from deploying a resource.
type ResourceDeployOutput struct {
	SpecState *core.MappingNode
}

// ResourceHasStabilisedInput provides the input data needed for a resource to
// determine if it has stabilised after being deployed.
type ResourceHasStabilisedInput struct {
	InstanceID string
	ResourceID string
	Params     core.BlueprintParams
}

// ResourceHasStabilisedOutput provides the output data from determining if a resource
// has stabilised after being deployed.
type ResourceHasStabilisedOutput struct {
	HasStabilised bool
}

// ResourceGetExternalStateInput provides the input data needed for a resource to
// get the external state of a resource.
type ResourceGetExternalStateInput struct {
	InstanceID string
	ResourceID string
	Params     core.BlueprintParams
}

// ResourceGetExternalStateOutput provides the output data from
// retrieving the external state of a resource.
type ResourceGetExternalStateOutput struct {
	State *state.ResourceState
}

// ResourceDestroyInput provides the input data needed to delete
// a resource.
type ResourceDestroyInput struct {
	InstanceID    string
	ResourceID    string
	ResourceState *state.ResourceState
	Params        core.BlueprintParams
}

// Changes provides a set of modified fields along with a version
// of the resource schema (includes metadata labels and annotations) and spec
// that has already had all it's variables substituted.
type Changes struct {
	// AppliedResourceInfo provides a new version of the spec
	// and schema for which variable substitution has been applied
	// so the deploy phase has everything it needs to deploy the resource.
	AppliedResourceInfo ResourceInfo  `json:"appliedResourceInfo"`
	MustRecreate        bool          `json:"mustRecreate"`
	ModifiedFields      []FieldChange `json:"modifiedFields"`
	NewFields           []FieldChange `json:"newFields"`
	RemovedFields       []string      `json:"removedFields"`
	UnchangedFields     []string      `json:"unchangedFields"`
	// ComputedFields holds a list of field names that are computed
	// at deploy time. This is primarily useful to give fast access to
	// information about which fields are computed without having to inspect
	// the spec schema in link implementations.
	ComputedFields []string `json:"computedFields"`
	// FieldChangesKnownOnDeploy holds a list of field names
	// for which changes will be known when the host blueprint is deployed.
	FieldChangesKnownOnDeploy []string `json:"fieldChangesKnownOnDeploy"`
	// ConditionKnownOnDeploy specifies whether the condition
	// for the resource will be known when the host blueprint is deployed.
	// When a condition makes use of items in the blueprint that are not resolved
	// until deployment, whether the resource will be deployed or not
	// cannot be known during the change staging phase.
	ConditionKnownOnDeploy bool `json:"conditionKnownOnDeploy"`
	// NewOutboundLinks holds a mapping of the linked to resource name
	// to the link changes representing the new links that will be created.
	NewOutboundLinks map[string]LinkChanges `json:"newOutboundLinks"`
	// OutboundLinkChanges holds a mapping
	// of the linked to resource name to any changes
	// that will be made to existing links.
	// The key is of the form `{resourceA}::{resoureB}`
	OutboundLinkChanges map[string]LinkChanges `json:"outboundLinkChanges"`
	// RemovedOutboundLinks holds a list of link identifiers
	// that will be removed.
	// The form of the link identifier is `{resourceA}::{resoureB}`
	RemovedOutboundLinks []string `json:"removedOutboundLinks"`
}

// ResourceSpecDefinition provides a definition for a resource spec
// that is used for state management, validation, docs and tooling.
type ResourceSpecDefinition struct {
	// Schema holds the schema for the resource
	// specification.
	Schema *ResourceDefinitionsSchema
	// IDField holds the name of the field in the resource spec
	// that holds the ID of the resource.
	// This is used to resolve references to a resource in a blueprint
	// where only the name of the resource is specified.
	// For example, references such as `resources.processOrderFunction` or `processOrderFunction`
	// should resolve to the ID of the resource in the blueprint.
	// The ID field must be a top-level property of the resource spec schema.
	IDField string
}

// ResourceDefinitionsSchema provides a schema that can be used to validate
// a resource spec or output state.
type ResourceDefinitionsSchema struct {
	// Type holds the type of the resource definition.
	Type ResourceDefinitionsSchemaType
	// Label holds a human-readable label for the resource definition.
	Label string
	// Description holds a human-readable description for the resource definition
	// without any formatting.
	Description string
	// FormattedDescription holds a human-readable description for the resource definition
	// that is formatted with markdown.
	FormattedDescription string
	// Attributes holds a mapping of the attribute types for a resource definition
	// schema object.
	Attributes map[string]*ResourceDefinitionsSchema
	// Items holds the schema for the items in a resource definition schema array.
	Items *ResourceDefinitionsSchema
	// MapValues holds the schema for the values in a resource definition schema map.
	// Keys are always strings.
	MapValues *ResourceDefinitionsSchema
	// OneOf holds a list of possible schemas for a resource definition item.
	// This is to be used with the "union" type.
	OneOf []*ResourceDefinitionsSchema
	// Required holds a list of required attributes for a resource definition schema object.
	Required []string
	// Nullable specifies whether the resource definition schema can be null.
	Nullable bool
	// Default holds the default value for a resource spec schema,
	// this will be populated in the `Resource.Spec.*` mapping node
	// if the resource spec is missing a value
	// for a specific attribute or item in the spec.
	// The default value will not be used if the attribute value in a given resource spec is nil
	// and the schema is nullable, a nil pointer should not be used
	// for an empty value, pointers should be set when you want to explicitly
	// set a value to nil.
	// The default value will not be used for computed value in a resource spec.
	Default *core.MappingNode
	// Computed specifies whether the value is computed by the provider
	// and should not be set by the user.
	// Computed values are expected to be populated by resource implementations
	// in a provider in the deployment process.
	Computed bool
	// MustRecreate specifies whether the resource must be recreated
	// if a change to the field is detected in the resource state.
	// This is only used for user-provided values, it will be ignored
	// for computed values.
	MustRecreate bool
}

// ResourceDefinitionsSchemaType holds the type of a resource schema.
type ResourceDefinitionsSchemaType string

const (
	// ResourceDefinitionsSchemaTypeString is for a schema string.
	ResourceDefinitionsSchemaTypeString ResourceDefinitionsSchemaType = "string"
	// ResourceDefinitionsSchemaTypeInteger is for a schema integer.
	ResourceDefinitionsSchemaTypeInteger ResourceDefinitionsSchemaType = "integer"
	// ResourceDefinitionsSchemaTypeFloat is for a schema float.
	ResourceDefinitionsSchemaTypeFloat ResourceDefinitionsSchemaType = "float"
	// ResourceDefinitionsSchemaTypeBoolean is for a schema boolean.
	ResourceDefinitionsSchemaTypeBoolean ResourceDefinitionsSchemaType = "boolean"
	// ResourceDefinitionsSchemaTypeMap is for a schema map.
	ResourceDefinitionsSchemaTypeMap ResourceDefinitionsSchemaType = "map"
	// ResourceDefinitionsSchemaTypeObject is for a schema object.
	ResourceDefinitionsSchemaTypeObject ResourceDefinitionsSchemaType = "object"
	// ResourceDefinitionsSchemaTypeArray is for a schema array.
	ResourceDefinitionsSchemaTypeArray ResourceDefinitionsSchemaType = "array"
	// ResourceDefinitionsSchemaTypeUnion is for an element that can be one of
	// multiple schemas.
	ResourceDefinitionsSchemaTypeUnion ResourceDefinitionsSchemaType = "union"
)
