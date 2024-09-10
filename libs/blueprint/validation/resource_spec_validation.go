package validation

import (
	"context"
	"fmt"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/source"
)

// ValidateResourceSpec validates the `spec` field of a resource.
// In a blueprint, this is a free form object that can hold complex structures
// that define a resource's configuration.
// Each resource has its own schema that is defined by the provider,
// this function validates against that schema and runs custom validation
// defined by the provider.
// This will only traverse up to `MappingNodeMaxTraverseDepth` levels deep,
// if the depth is exceeded, validation will not be performed on further elements.
func ValidateResourceSpec(
	ctx context.Context,
	name string,
	resourceType string,
	resource *schema.Resource,
	resourceLocation *source.Meta,
	bpSchema *schema.Blueprint,
	params core.BlueprintParams,
	funcRegistry provider.FunctionRegistry,
	refChainCollector RefChainCollector,
	resourceRegistry resourcehelpers.Registry,
) ([]*core.Diagnostic, error) {
	diagnostics := []*core.Diagnostic{}

	specDefinition, err := loadResourceSpecDefinition(
		ctx,
		resource.Type.Value,
		name,
		resource.SourceMeta,
		params,
		resourceRegistry,
	)
	if err != nil {
		// If there is no spec definition for the resource type,
		// we can't validate the spec.
		return diagnostics, err
	}

	var errs []error
	path := fmt.Sprintf("resources.%s.spec", name)
	specDiagnostics, err := validateResourceDefinition(
		ctx,
		name,
		resourceType,
		resource.Spec,
		resourceLocation,
		specDefinition.Schema,
		bpSchema,
		params,
		funcRegistry,
		refChainCollector,
		resourceRegistry,
		path,
		/* depth */ 0,
	)
	diagnostics = append(diagnostics, specDiagnostics...)
	if err != nil {
		errs = append(errs, err)
	}

	customOutput, err := resourceRegistry.CustomValidate(
		ctx,
		resourceType,
		&provider.ResourceValidateInput{
			SchemaResource: resource,
			Params:         params,
		},
	)
	if customOutput != nil {
		diagnostics = append(diagnostics, customOutput.Diagnostics...)
	}
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return diagnostics, ErrMultipleValidationErrors(errs)
	}

	return diagnostics, nil
}

func loadResourceSpecDefinition(
	ctx context.Context,
	resourceType string,
	resourceName string,
	location *source.Meta,
	params core.BlueprintParams,
	resourceRegistry resourcehelpers.Registry,
) (*provider.ResourceSpecDefinition, error) {
	specDefOutput, err := resourceRegistry.GetSpecDefinition(
		ctx,
		resourceType,
		&provider.ResourceGetSpecDefinitionInput{
			Params: params,
		},
	)
	if err != nil {
		return nil, errResourceTypeMissingSpecDefinition(
			resourceName,
			resourceType,
			/* inSubstitution */ false,
			location,
			"failed to load spec definition",
		)
	}

	if specDefOutput.SpecDefinition == nil {
		return nil, errResourceTypeMissingSpecDefinition(
			resourceName,
			resourceType,
			/* inSubstitution */ false,
			location,
			"spec definition is nil",
		)
	}

	if specDefOutput.SpecDefinition.Schema == nil {
		return nil, errResourceTypeSpecDefMissingSchema(
			resourceName,
			resourceType,
			/* inSubstitution */ false,
			location,
		)
	}

	return specDefOutput.SpecDefinition, nil
}
