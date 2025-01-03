package container

import (
	"context"
	"os"
	"slices"
	"strings"

	bpcore "github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/corefunctions"
	"github.com/two-hundred/celerity/libs/blueprint/includes"
	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/providerhelpers"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/source"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/subengine"
	"github.com/two-hundred/celerity/libs/blueprint/transform"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
	"github.com/two-hundred/celerity/libs/common/core"
)

// Loader provides the interface for a service that deals
// with loading blueprints for which instances can be deployed.
// The loader also provides methods for validating a spec
// (and optionally its variables) without associating
// it with an instance.
type Loader interface {
	// Load deals with loading a blueprint specification from the local file system
	// along with provider and blueprint variables.
	// Provider and blueprint variables can be provided to the blueprint container
	// methods at a later stage, you can provide an empty set of parameters when
	// loading a spec.
	Load(
		ctx context.Context,
		blueprintSpecFile string,
		params bpcore.BlueprintParams,
	) (BlueprintContainer, error)

	// Validate deals with validating a specification that lies on the local
	// file system without loading a blueprint container.
	// Provider and blueprint variables can be provided for enhanced
	// validation that also checks variables.
	//
	// This also returns validation diagnostics for warning and info level
	// diagnostics that point out potential issues that may occur when executing
	// a blueprint. Diagnostics do not include errors, errors should be unpacked from
	// the returned error.
	Validate(
		ctx context.Context,
		blueprintSpecFile string,
		params bpcore.BlueprintParams,
	) (*ValidationResult, error)

	// LoadString deals with loading a blueprint specification from a string
	// along with provider and blueprint variables.
	// Provider and blueprint variables can be provided to the blueprint container
	// methods at a later stage, you can provide an empty set of parameters when
	// loading a spec.
	LoadString(
		ctx context.Context,
		blueprintSpec string,
		inputFormat schema.SpecFormat,
		params bpcore.BlueprintParams,
	) (BlueprintContainer, error)

	// ValidateString deals with validating a specification provided as a string
	// without loading a blueprint container.
	// Provider and blueprint variables can be provided for enhanced
	// validation that also checks variables.
	//
	// This also returns validation diagnostics for error, warning and info level
	// diagnostics that point out potential issues that may occur when executing
	// a blueprint. Diagnostics do not include all errors, errors should be unpacked from
	// the returned error in addition to the diagnostics.
	ValidateString(
		ctx context.Context,
		blueprintSpec string,
		inputFormat schema.SpecFormat,
		params bpcore.BlueprintParams,
	) (*ValidationResult, error)

	// LoadFromSchema deals with loading a blueprint specification from a schema
	// that has already been parsed along with provider and blueprint variables.
	// This is mostly useful for loading a blueprint from a schema cache to speed up
	// loading times, this is especially useful for blueprints that make use of a lot
	// ${..} substitutions and references.
	// Provider and blueprint variables can be provided to the blueprint container
	// methods at a later stage, you can provide an empty set of parameters when
	// loading a spec.
	LoadFromSchema(
		ctx context.Context,
		blueprintSchema *schema.Blueprint,
		params bpcore.BlueprintParams,
	) (BlueprintContainer, error)

	// ValidateFromSchema deals with validating a specification provided as a schema
	// without loading a blueprint container.
	// Provider and blueprint variables can be provided for enhanced
	// validation that also checks variables.
	//
	// This also returns validation diagnostics for error, warning and info level
	// diagnostics that point out potential issues that may occur when executing
	// a blueprint. Diagnostics do not include all errors, errors should be unpacked from
	// the returned error in addition to the diagnostics.
	ValidateFromSchema(
		ctx context.Context,
		blueprintSchema *schema.Blueprint,
		params bpcore.BlueprintParams,
	) (*ValidationResult, error)
}

// ValidationResult provides information about the result of validating
// a blueprint.
type ValidationResult struct {
	// Collected diagnostics from the validation process.
	Diagnostics []*bpcore.Diagnostic
	// The link information that was collected during validation.
	LinkInfo links.SpecLinkInfo
	// The parsed blueprint schema that was validated.
	Schema *schema.Blueprint
}

type loadBlueprintInfo struct {
	specOrFilePath  string
	preloadedSchema *schema.Blueprint
}

// Stores the full blueprint schema and direct access to the
// mapping of resource names to their schemas for convenience.
// This is structure of the spec encapsulated by the blueprint container.
type internalBlueprintSpec struct {
	resourceSchemas map[string]*schema.Resource
	schema          *schema.Blueprint
}

func (s *internalBlueprintSpec) ResourceSchema(resourceName string) *schema.Resource {
	resourceSchema, ok := s.resourceSchemas[resourceName]
	if !ok {
		return nil
	}
	return resourceSchema
}

func (s *internalBlueprintSpec) Schema() *schema.Blueprint {
	return s.schema
}

type defaultLoader struct {
	providers              map[string]provider.Provider
	specTransformers       map[string]transform.SpecTransformer
	stateContainer         state.Container
	resourceChangeStager   ResourceChangeStager
	childResolver          includes.ChildResolver
	validateRuntimeValues  bool
	validateAfterTransform bool
	transformSpec          bool
	// A list of resource names derived from resource templates.
	// "elem" and "i" references should be allowed in resources
	// derived from templates where the `each` property is not set.
	derivedFromTemplates      []string
	refChainCollectorFactory  func() validation.RefChainCollector
	funcRegistry              provider.FunctionRegistry
	resourceRegistry          resourcehelpers.Registry
	dataSourceRegistry        provider.DataSourceRegistry
	linkRegistry              provider.LinkRegistry
	clock                     bpcore.Clock
	resolveWorkingDir         corefunctions.WorkingDirResolver
	idGenerator               bpcore.IDGenerator
	defaultRetryPolicy        *provider.RetryPolicy
	deploymentStateFactory    DeploymentStateFactory
	changeStagingStateFactory ChangeStagingStateFactory
	// A mapping of resource names to the templates they were derived from.
	// This is only populated for a loader when loading a derived blueprint
	// where templates are not used in the derived blueprint but were
	// used in the original.
	// This is primarily useful for rollback operations where a simplified
	// blueprint is derived from the previous state of a blueprint instance.
	resourceTemplates map[string]string
}

type LoaderOption func(loader *defaultLoader)

// WithLoaderValidateRuntimeValues sets the flag to determine whether
// runtime values should be validated when loading blueprints.
// This is useful when you want to validate a blueprint spec without
// associating it with an instance.
// (e.g. validation for code editors or CLI dry runs)
//
// When this option is not provided, the default value is false.
func WithLoaderValidateRuntimeValues(validateRuntimeValues bool) LoaderOption {
	return func(loader *defaultLoader) {
		loader.validateRuntimeValues = validateRuntimeValues
	}
}

// WithLoaderValidateAfterTransform sets the flag to determine whether
// resource validation should be performed after applying transformers to the
// blueprint spec.
// This is useful when you want to catch potential bugs in transformer implementations
// at the load/validate stage.
//
// When this option is not provided, the default value is false.
func WithLoaderValidateAfterTransform(validateAfterTransform bool) LoaderOption {
	return func(loader *defaultLoader) {
		loader.validateAfterTransform = validateAfterTransform
	}
}

// WithLoaderTransformSpec sets the flag to determine whether transformers should be applied
// to the blueprint spec when loading blueprints.
// This is useful when you want to validate a blueprint spec without
// applying any transformations.
// (e.g. validation for code editors or CLI dry runs)
//
// When this option is not provided, the default value is true.
func WithLoaderTransformSpec(transformSpec bool) LoaderOption {
	return func(loader *defaultLoader) {
		loader.transformSpec = transformSpec
	}
}

// WithLoaderClock sets the clock to be used by the loader.
//
// When this option is not provided, the default value is the system clock.
func WithLoaderClock(clock bpcore.Clock) LoaderOption {
	return func(loader *defaultLoader) {
		loader.clock = clock
	}
}

// WithLoaderResolveWorkingDir sets the function to resolve the working directory
// to be used by the loader.
//
// When this option is not provided, the default value is os.Getwd.
func WithLoaderResolveWorkingDir(resolveWorkingDir corefunctions.WorkingDirResolver) LoaderOption {
	return func(loader *defaultLoader) {
		loader.resolveWorkingDir = resolveWorkingDir
	}
}

// WithLoaderDerivedFromTemplates sets the list of resource names that are derived
// from resource templates.
// This is useful when you want to allow references to "elem" and "i" in resources
// derived from templates where the `each` property is not set.
func WithLoaderDerivedFromTemplates(derivedFromTemplates []string) LoaderOption {
	return func(loader *defaultLoader) {
		loader.derivedFromTemplates = derivedFromTemplates
	}
}

// WithLoaderIDGenerator sets the ID generator to be used by blueprint containers
// created by the loader in the deployment process.
//
// When this option is not provided, a UUID generator is used.
func WithLoaderIDGenerator(idGenerator bpcore.IDGenerator) LoaderOption {
	return func(loader *defaultLoader) {
		loader.idGenerator = idGenerator
	}
}

// WithLoaderDefaultRetryPolicy sets the default retry policy to be used for deployments
// in blueprint containers created by the loader.
func WithLoaderDefaultRetryPolicy(retryPolicy *provider.RetryPolicy) LoaderOption {
	return func(loader *defaultLoader) {
		loader.defaultRetryPolicy = retryPolicy
	}
}

// WithLoaderResourceRegistry sets the resource registry to be used by the loader.
//
// When this option is not provided, the default resource registry that is
// derived from the given providers and transformers is used.
func WithLoaderResourceRegistry(resourceRegistry resourcehelpers.Registry) LoaderOption {
	return func(loader *defaultLoader) {
		loader.resourceRegistry = resourceRegistry
	}
}

// WithLoaderFunctionRegistry sets the function registry to be used by the loader.
//
// When this option is not provided, the default function registry that is
// derived from the given providers is used.
func WithLoaderFunctionRegistry(funcRegistry provider.FunctionRegistry) LoaderOption {
	return func(loader *defaultLoader) {
		loader.funcRegistry = funcRegistry
	}
}

// WithLoaderDataSourceRegistry sets the data source registry to be used by the loader.
//
// When this option is not provided, the default data source registry that is
// derived from the given providers is used.
func WithLoaderDataSourceRegistry(dataSourceRegistry provider.DataSourceRegistry) LoaderOption {
	return func(loader *defaultLoader) {
		loader.dataSourceRegistry = dataSourceRegistry
	}
}

// WithLoaderLinkRegistry sets the link registry to be used by the loader.
//
// When this option is not provided, the default link registry that is
// derived from the given providers is used.
func WithLoaderLinkRegistry(linkRegistry provider.LinkRegistry) LoaderOption {
	return func(loader *defaultLoader) {
		loader.linkRegistry = linkRegistry
	}
}

// WithLoaderDeploymentStateFactory sets the deployment state factory to be used by the loader.
//
// When this option is not provided, the default deployment state factory is used
// to create an ephemeral, thread-safe store for deployment state.
func WithLoaderDeploymentStateFactory(deploymentStateFactory DeploymentStateFactory) LoaderOption {
	return func(loader *defaultLoader) {
		loader.deploymentStateFactory = deploymentStateFactory
	}
}

// WithLoaderChangeStagingStateFactory sets the change staging state factory to be used by the loader.
//
// When this option is not provided, the default change staging state factory is used
// to create an ephemeral, thread-safe store for staging changes.
func WithLoaderChangeStagingStateFactory(changeStagingStateFactory ChangeStagingStateFactory) LoaderOption {
	return func(loader *defaultLoader) {
		loader.changeStagingStateFactory = changeStagingStateFactory
	}
}

// WithLoaderResourceTemplates sets the resource templates to be used by the loader.
// Resource templates are a mapping of resource names to the templates they were derived from.
// This is useful when loading a derived blueprint where templates are not used but were
// in the original.
// This is primarily useful for rollback operations where a simplified
// blueprint is derived from the previous state of a blueprint instance.
//
// When this option is not provided, an empty map will be used.
func WithLoaderResourceTemplates(resourceTemplates map[string]string) LoaderOption {
	return func(loader *defaultLoader) {
		loader.resourceTemplates = resourceTemplates
	}
}

// WithLoaderResourceChangeStager sets the resource change stager to be used in blueprint
// containers created by the loader.
//
// When this option is not provided, the default resource change stager is used.
func WithLoaderResourceChangeStager(changeStager ResourceChangeStager) LoaderOption {
	return func(loader *defaultLoader) {
		loader.resourceChangeStager = changeStager
	}
}

// WithLoaderRefChainCollectorFactory sets the reference chain collector factory to be used by the loader.
//
// When this option is not provided, the default reference chain collector factory
// provided by the validation package is used.
func WithLoaderRefChainCollectorFactory(factory func() validation.RefChainCollector) LoaderOption {
	return func(loader *defaultLoader) {
		loader.refChainCollectorFactory = factory
	}
}

// NewDefaultLoader creates a new instance of the default
// implementation of a blueprint container loader.
// The map of providers must be a map of provider namespaces
// to the implementation.
// For example, for all resource types "aws/*" you would have a mapping
// namespace "aws" to the AWS provider.
// The namespace must be the prefix of resource, data source and custom
// variable types defined by the provider.
// If there is no provider for the prefix of a resource, data source or
// custom variable type in a blueprint, it will fail.
//
// You can provide options for multiple flags that can be set to determine
// how the loader should behave, such as whether to validate runtime values
// or validate after transformation when loading blueprints.
func NewDefaultLoader(
	providers map[string]provider.Provider,
	specTransformers map[string]transform.SpecTransformer,
	stateContainer state.Container,
	childResolver includes.ChildResolver,
	opts ...LoaderOption,
) Loader {
	resourceRegistry := resourcehelpers.NewRegistry(providers, specTransformers)
	funcRegistry := provider.NewFunctionRegistry(providers)
	dataSourceRegistry := provider.NewDataSourceRegistry(providers)
	linkRegistry := provider.NewLinkRegistry(providers)
	internalProviders := copyProviderMap(providers)

	loader := &defaultLoader{
		providers:                 internalProviders,
		specTransformers:          specTransformers,
		stateContainer:            stateContainer,
		resourceChangeStager:      NewDefaultResourceChangeStager(),
		childResolver:             childResolver,
		refChainCollectorFactory:  validation.NewRefChainCollector,
		funcRegistry:              funcRegistry,
		resourceRegistry:          resourceRegistry,
		dataSourceRegistry:        dataSourceRegistry,
		linkRegistry:              linkRegistry,
		clock:                     &bpcore.SystemClock{},
		resolveWorkingDir:         os.Getwd,
		derivedFromTemplates:      []string{},
		idGenerator:               bpcore.NewUUIDGenerator(),
		defaultRetryPolicy:        provider.DefaultRetryPolicy,
		deploymentStateFactory:    NewDefaultDeploymentState,
		changeStagingStateFactory: NewDefaultChangeStagingState,
		resourceTemplates:         map[string]string{},
	}

	for _, opt := range opts {
		opt(loader)
	}

	if _, hasCore := internalProviders["core"]; !hasCore {
		internalProviders["core"] = providerhelpers.NewCoreProvider(
			stateContainer.Links(),
			bpcore.BlueprintInstanceIDFromContext,
			loader.resolveWorkingDir,
			loader.clock,
		)
	}

	return loader
}

func (l *defaultLoader) forChildBlueprint(
	derivedFromTemplate []string,
	resourceTemplates map[string]string,
) Loader {
	return NewDefaultLoader(
		l.providers,
		l.specTransformers,
		l.stateContainer,
		l.childResolver,
		WithLoaderValidateRuntimeValues(l.validateRuntimeValues),
		WithLoaderValidateAfterTransform(l.validateAfterTransform),
		WithLoaderTransformSpec(l.transformSpec),
		WithLoaderClock(l.clock),
		WithLoaderResolveWorkingDir(l.resolveWorkingDir),
		WithLoaderDerivedFromTemplates(derivedFromTemplate),
		WithLoaderIDGenerator(l.idGenerator),
		WithLoaderDefaultRetryPolicy(l.defaultRetryPolicy),
		WithLoaderResourceRegistry(l.resourceRegistry),
		WithLoaderFunctionRegistry(l.funcRegistry),
		WithLoaderDataSourceRegistry(l.dataSourceRegistry),
		WithLoaderLinkRegistry(l.linkRegistry),
		WithLoaderDeploymentStateFactory(l.deploymentStateFactory),
		WithLoaderChangeStagingStateFactory(l.changeStagingStateFactory),
		WithLoaderResourceTemplates(resourceTemplates),
		WithLoaderResourceChangeStager(l.resourceChangeStager),
		WithLoaderRefChainCollectorFactory(l.refChainCollectorFactory),
	)
}

func (l *defaultLoader) Load(ctx context.Context, blueprintSpecFile string, params bpcore.BlueprintParams) (BlueprintContainer, error) {
	loadInfo := &loadBlueprintInfo{
		specOrFilePath: blueprintSpecFile,
	}
	container, _, err := l.loadSpecAndLinkInfo(ctx, loadInfo, params, schema.Load, deriveSpecFormat)
	return container, err
}

func (l *defaultLoader) Validate(
	ctx context.Context,
	blueprintSpecFile string,
	params bpcore.BlueprintParams,
) (*ValidationResult, error) {
	loadInfo := &loadBlueprintInfo{
		specOrFilePath: blueprintSpecFile,
	}
	container, diagnostics, err := l.loadSpecAndLinkInfo(ctx, loadInfo, params, schema.Load, deriveSpecFormat)
	if err != nil {
		return &ValidationResult{
			Diagnostics: diagnostics,
			Schema:      getSchemaFromContainer(container),
		}, err
	}
	return &ValidationResult{
		Diagnostics: diagnostics,
		Schema:      getSchemaFromContainer(container),
		LinkInfo:    container.SpecLinkInfo(),
	}, nil
}

func (l *defaultLoader) loadSpecAndLinkInfo(
	ctx context.Context,
	loadInfo *loadBlueprintInfo,
	params bpcore.BlueprintParams,
	schemaLoader schema.Loader,
	formatLoader func(string) (schema.SpecFormat, error),
) (BlueprintContainer, []*bpcore.Diagnostic, error) {
	refChainCollector := l.refChainCollectorFactory()
	blueprintSpec, diagnostics, err := l.loadSpec(ctx, loadInfo, params, schemaLoader, formatLoader, refChainCollector)
	if err != nil {
		// Ensure the spec is returned when parsing was successful
		// but validation failed.
		return NewDefaultBlueprintContainer(
			blueprintSpec,
			l.buildPartialBlueprintContainerDependencies(refChainCollector),
			diagnostics,
		), diagnostics, err
	}

	resourceTypeProviderMap := createResourceTypeProviderMap(blueprintSpec, l.providers)
	linkInfo, err := links.NewDefaultLinkInfoProvider(resourceTypeProviderMap, l.linkRegistry, blueprintSpec, params)
	if err != nil {
		// Ensure the spec is returned when parsing and
		// validation was successful but loading link information failed.
		return NewDefaultBlueprintContainer(
			blueprintSpec,
			l.buildPartialBlueprintContainerDependencies(refChainCollector),
			diagnostics,
		), diagnostics, err
	}

	container := NewDefaultBlueprintContainer(
		blueprintSpec,
		l.buildFullBlueprintContainerDependencies(
			refChainCollector,
			blueprintSpec,
			linkInfo,
			params,
		),
		diagnostics,
	)

	// Once we have loaded the link information,
	// we can capture links as references to include in checks
	// for reference/link cycles to catch the case where a resource selects another through
	// a link and the other resource references a property of the first resource.
	err = l.collectLinksAsReferences(ctx, linkInfo, refChainCollector)
	if err != nil {
		return container, diagnostics, err
	}

	refCycleRoots := refChainCollector.FindCircularReferences()
	if len(refCycleRoots) > 0 {
		return container, diagnostics, validation.ErrReferenceCycles(refCycleRoots)
	}

	eachDepsErr := validation.ValidateResourceEachDependencies(
		blueprintSpec.Schema(),
		refChainCollector,
	)
	if eachDepsErr != nil {
		return container, diagnostics, eachDepsErr
	}

	return container, diagnostics, nil
}

func (l *defaultLoader) buildPartialBlueprintContainerDependencies(
	refChainCollector validation.RefChainCollector,
) *BlueprintContainerDependencies {
	return &BlueprintContainerDependencies{
		StateContainer:              l.stateContainer,
		Providers:                   map[string]provider.Provider{},
		ResourceRegistry:            l.resourceRegistry,
		LinkRegistry:                l.linkRegistry,
		LinkInfo:                    nil,
		ResourceTemplates:           l.resourceTemplates,
		RefChainCollector:           refChainCollector,
		SubstitutionResolver:        nil,
		ChildBlueprintLoaderFactory: l.forChildBlueprint,
		Clock:                       l.clock,
		IDGenerator:                 l.idGenerator,
		DefaultRetryPolicy:          l.defaultRetryPolicy,
		DeploymentStateFactory:      l.deploymentStateFactory,
		ChangeStagingStateFactory:   l.changeStagingStateFactory,
	}
}

func (l *defaultLoader) buildFullBlueprintContainerDependencies(
	refChainCollector validation.RefChainCollector,
	blueprintSpec *internalBlueprintSpec,
	linkInfo links.SpecLinkInfo,
	params bpcore.BlueprintParams,
) *BlueprintContainerDependencies {
	resourceCache := bpcore.NewCache[*provider.ResolvedResource]()
	resourceTemplateInputElemCache := bpcore.NewCache[[]*bpcore.MappingNode]()
	childExportFieldCache := bpcore.NewCache[*subengine.ChildExportFieldInfo]()
	substitutionResolver := subengine.NewDefaultSubstitutionResolver(
		&subengine.Registries{
			FuncRegistry:       l.funcRegistry,
			ResourceRegistry:   l.resourceRegistry,
			DataSourceRegistry: l.dataSourceRegistry,
		},
		l.stateContainer,
		resourceCache,
		resourceTemplateInputElemCache,
		childExportFieldCache,
		blueprintSpec,
		params,
	)
	blueprintPreparer := NewDefaultBlueprintPreparer(
		l.providers,
		substitutionResolver,
		resourceTemplateInputElemCache,
		l.resourceRegistry,
		resourceCache,
		l.forChildBlueprint,
	)
	// As the link change stager uses the resource cache and substitution resolver,
	// it must be created for each blueprint container that is loaded.
	linkChangeStager := NewDefaultLinkChangeStager(
		l.stateContainer,
		substitutionResolver,
		resourceCache,
	)
	// As the child change stager uses the child export field cache and
	// substitution resolver, it must be created for each blueprint container that is loaded.
	childChangeStager := NewDefaultChildChangeStager(
		l.childResolver,
		l.forChildBlueprint,
		l.stateContainer,
		childExportFieldCache,
		substitutionResolver,
	)

	return &BlueprintContainerDependencies{
		StateContainer:                 l.stateContainer,
		Providers:                      l.providers,
		ResourceRegistry:               l.resourceRegistry,
		LinkInfo:                       linkInfo,
		ResourceTemplates:              l.resourceTemplates,
		LinkRegistry:                   l.linkRegistry,
		RefChainCollector:              refChainCollector,
		SubstitutionResolver:           substitutionResolver,
		ChangeStager:                   l.resourceChangeStager,
		ChildResolver:                  l.childResolver,
		ResourceCache:                  resourceCache,
		ResourceTemplateInputElemCache: resourceTemplateInputElemCache,
		ChildExportFieldCache:          childExportFieldCache,
		ChildBlueprintLoaderFactory:    l.forChildBlueprint,
		Clock:                          l.clock,
		IDGenerator:                    l.idGenerator,
		DefaultRetryPolicy:             l.defaultRetryPolicy,
		DeploymentStateFactory:         l.deploymentStateFactory,
		ChangeStagingStateFactory:      l.changeStagingStateFactory,
		BlueprintPreparer:              blueprintPreparer,
		LinkChangeStager:               linkChangeStager,
		ChildChangeStager:              childChangeStager,
	}
}

func (l *defaultLoader) collectLinksAsReferences(
	ctx context.Context,
	linkInfo links.SpecLinkInfo,
	refChainCollector validation.RefChainCollector,
) error {
	chains, err := linkInfo.Links(ctx)
	if err != nil {
		return err
	}

	for _, chain := range chains {
		err = collectLinksFromChain(ctx, chain, refChainCollector)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *defaultLoader) loadSpec(
	ctx context.Context,
	loadInfo *loadBlueprintInfo,
	params bpcore.BlueprintParams,
	loader schema.Loader,
	formatLoader func(string) (schema.SpecFormat, error),
	refChainCollector validation.RefChainCollector,
) (*internalBlueprintSpec, []*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}

	blueprintSchema, err := loadBlueprintSpec(loadInfo, formatLoader, loader)
	if err != nil {
		return &internalBlueprintSpec{}, diagnostics, err
	}

	var bpValidationDiagnostics []*bpcore.Diagnostic
	validationErrors := []error{}
	bpValidationDiagnostics, err = l.validateBlueprint(ctx, blueprintSchema)
	diagnostics = append(diagnostics, bpValidationDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var variableDiagnostics []*bpcore.Diagnostic
	variableDiagnostics, err = l.validateVariables(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, variableDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var valueDiagnostics []*bpcore.Diagnostic
	valueDiagnostics, err = l.validateValues(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, valueDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var includeDiagnostics []*bpcore.Diagnostic
	includeDiagnostics, err = l.validateIncludes(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, includeDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var exportDiagnostics []*bpcore.Diagnostic
	exportDiagnostics, err = l.validateExports(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, exportDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var metadataDiagnostics []*bpcore.Diagnostic
	metadataDiagnostics, err = l.validateMetadata(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, metadataDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var dataSourceDiagnostics []*bpcore.Diagnostic
	dataSourceDiagnostics, err = l.validateDataSources(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, dataSourceDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	var resourceDiagnostics []*bpcore.Diagnostic
	resourceDiagnostics, err = l.validateResources(ctx, blueprintSchema, params, refChainCollector)
	diagnostics = append(diagnostics, resourceDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	transformers, transformDiagnostics, err := l.validateAndApplyTransforms(ctx, blueprintSchema)
	diagnostics = append(diagnostics, transformDiagnostics...)
	if err != nil {
		return &internalBlueprintSpec{
			schema: blueprintSchema,
		}, diagnostics, err
	}

	if l.validateAfterTransform && len(transformers) > 0 {
		// Validate after transformations to help with catching bugs in transformer implementations.
		// This ultimately prevents transformers from expanding their abstractions into invalid
		// representations of the lower level resources.
		var resourceDiagnostics []*bpcore.Diagnostic
		resourceDiagnostics, err = l.validateResources(ctx, blueprintSchema, params, refChainCollector)
		diagnostics = append(diagnostics, resourceDiagnostics...)
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return &internalBlueprintSpec{
			schema: blueprintSchema,
		}, diagnostics, validation.ErrMultipleValidationErrors(validationErrors)
	}

	return &internalBlueprintSpec{
		schema: blueprintSchema,
	}, diagnostics, nil
}

func (l *defaultLoader) validateAndApplyTransforms(
	ctx context.Context,
	blueprintSchema *schema.Blueprint,
) ([]transform.SpecTransformer, []*bpcore.Diagnostic, error) {
	// Apply some validation to get diagnostics about non-standard transformers
	// that may not be present at runtime.
	var transformDiagnostics []*bpcore.Diagnostic
	var validationErrors []error
	validateDiagnostics, err := l.validateTransforms(ctx, blueprintSchema)
	transformDiagnostics = append(transformDiagnostics, validateDiagnostics...)
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	if !l.transformSpec {
		if len(validationErrors) > 0 {
			return nil, transformDiagnostics, validation.ErrMultipleValidationErrors(validationErrors)
		}
		return nil, transformDiagnostics, nil
	}

	transformers, err := l.collectTransformers(blueprintSchema)
	if err != nil {
		return nil, transformDiagnostics, validation.ErrMultipleValidationErrors(
			append(validationErrors, err),
		)
	}
	for _, transformer := range transformers {
		output, err := transformer.Transform(ctx, &transform.SpecTransformerTransformInput{
			InputBlueprint: blueprintSchema,
		})
		blueprintSchema = output.TransformedBlueprint
		if err != nil {
			return transformers, transformDiagnostics, validation.ErrMultipleValidationErrors(
				append(validationErrors, err),
			)
		}
	}

	return transformers, transformDiagnostics, nil
}

func (l *defaultLoader) collectTransformers(schema *schema.Blueprint) ([]transform.SpecTransformer, error) {
	usedBySpec := []transform.SpecTransformer{}
	missingTransformers := []string{}
	childErrors := []error{}

	if schema.Transform == nil {
		return usedBySpec, nil
	}

	for i, name := range schema.Transform.Values {
		transformer, exists := l.specTransformers[name]
		if exists {
			usedBySpec = append(usedBySpec, transformer)
		} else {
			missingTransformers = append(missingTransformers, name)
			sourceMeta := (*source.Meta)(nil)
			if len(schema.Transform.SourceMeta) > 0 {
				sourceMeta = schema.Transform.SourceMeta[i]
			}
			line, col := source.PositionFromSourceMeta(sourceMeta)
			childErrors = append(childErrors, errTransformerMissing(name, line, col))
		}
	}
	if len(missingTransformers) > 0 {
		firstSourceMeta := (*source.Meta)(nil)
		if len(schema.Transform.SourceMeta) > 0 {
			firstSourceMeta = schema.Transform.SourceMeta[0]
		}

		line, col := source.PositionFromSourceMeta(firstSourceMeta)
		return nil, errTransformersMissing(missingTransformers, childErrors, line, col)
	}
	return usedBySpec, nil
}

func (l *defaultLoader) validateBlueprint(ctx context.Context, bpSchema *schema.Blueprint) ([]*bpcore.Diagnostic, error) {
	return validation.ValidateBlueprint(ctx, bpSchema)
}

func (l *defaultLoader) validateTransforms(ctx context.Context, bpSchema *schema.Blueprint) ([]*bpcore.Diagnostic, error) {
	return validation.ValidateTransforms(ctx, bpSchema, l.transformSpec)
}

func (l *defaultLoader) validateVariables(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.Variables == nil {
		return diagnostics, nil
	}

	// To be as useful as possible, we'll collect and
	// report issues for all the problematic variables.
	variableErrors := map[string][]error{}
	for name, varSchema := range bpSchema.Variables.Values {
		currentVarErrs := l.validateVariable(ctx, &diagnostics, name, varSchema, bpSchema, params, refChainCollector)
		if len(currentVarErrs) > 0 {
			variableErrors[name] = currentVarErrs
		}
	}

	if len(variableErrors) > 0 {
		return diagnostics, errVariableValidationError(variableErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateVariable(
	ctx context.Context,
	diagnostics *[]*bpcore.Diagnostic,
	name string,
	varSchema *schema.Variable,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) []error {
	currentVarErrs := []error{}
	err := validation.ValidateVariableName(name, bpSchema.Variables)
	if err != nil {
		currentVarErrs = append(currentVarErrs, err)
	}

	if varSchema.Type == nil {
		currentVarErrs = append(currentVarErrs, errMissingVariableType(name, varSchema.SourceMeta))
		return currentVarErrs
	}

	if core.SliceContains(schema.CoreVariableTypes, varSchema.Type.Value) {
		coreVarDiagnostics, err := validation.ValidateCoreVariable(
			ctx, name, varSchema, bpSchema.Variables, params, l.validateRuntimeValues,
		)
		if err != nil {
			currentVarErrs = append(currentVarErrs, err)
		}
		*diagnostics = append(*diagnostics, coreVarDiagnostics...)
	} else {
		customVarDiagnostics, err := l.validateCustomVariableType(ctx, name, varSchema, bpSchema.Variables, params)
		if err != nil {
			currentVarErrs = append(currentVarErrs, err)
		}
		*diagnostics = append(*diagnostics, customVarDiagnostics...)
	}

	refChainCollector.Collect(bpcore.VariableElementID(name), varSchema, "", []string{})
	return currentVarErrs
}

func (l *defaultLoader) validateValues(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.Values == nil {
		return diagnostics, nil
	}

	valueErrors := map[string][]error{}
	for name, valSchema := range bpSchema.Values.Values {
		currentValErrs := l.validateValue(ctx, &diagnostics, name, valSchema, bpSchema, params, refChainCollector)
		if len(currentValErrs) > 0 {
			valueErrors[name] = currentValErrs
		}
	}

	if len(valueErrors) > 0 {
		return diagnostics, errVariableValidationError(valueErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateValue(
	ctx context.Context,
	diagnostics *[]*bpcore.Diagnostic,
	name string,
	valSchema *schema.Value,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) []error {
	currentValErrs := []error{}
	err := validation.ValidateValueName(name, bpSchema.Values)
	if err != nil {
		currentValErrs = append(currentValErrs, err)
	}

	resultDiagnostics, err := validation.ValidateValue(
		ctx,
		name,
		valSchema,
		bpSchema,
		params,
		l.funcRegistry,
		refChainCollector,
		l.resourceRegistry,
	)
	if err != nil {
		currentValErrs = append(currentValErrs, err)
	}
	*diagnostics = append(*diagnostics, resultDiagnostics...)

	refChainCollector.Collect(bpcore.ValueElementID(name), valSchema, "", []string{})
	return currentValErrs
}

func (l *defaultLoader) validateIncludes(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.Include == nil {
		return diagnostics, nil
	}

	// We'll collect and report issues for all the problematic includes.
	includeErrors := map[string]error{}
	for name, includeSchema := range bpSchema.Include.Values {
		includeDiagnostics, err := validation.ValidateInclude(
			ctx,
			name,
			includeSchema,
			bpSchema.Include,
			bpSchema,
			params,
			l.funcRegistry,
			refChainCollector,
			l.resourceRegistry,
		)
		if err != nil {
			includeErrors[name] = err
		}
		diagnostics = append(diagnostics, includeDiagnostics...)

		refChainCollector.Collect(bpcore.ChildElementID(name), includeSchema, "", []string{})
	}

	if len(includeErrors) > 0 {
		return diagnostics, errIncludeValidationError(includeErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateExports(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.Exports == nil {
		return diagnostics, nil
	}

	// We'll collect and report issues for all the problematic exports.
	exportErrors := map[string]error{}
	for name, exportSchema := range bpSchema.Exports.Values {
		exportDiagnostics, err := validation.ValidateExport(
			ctx,
			name,
			exportSchema,
			bpSchema.Exports,
			bpSchema,
			params,
			l.funcRegistry,
			refChainCollector,
			l.resourceRegistry,
		)
		if err != nil {
			exportErrors[name] = err
		}
		diagnostics = append(diagnostics, exportDiagnostics...)
	}

	if len(exportErrors) > 0 {
		return diagnostics, errExportValidationError(exportErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateMetadata(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	if bpSchema.Metadata == nil {
		return []*bpcore.Diagnostic{}, nil
	}

	return validation.ValidateMappingNode(
		ctx,
		"root",
		"metadata",
		/* usedInResourceDerivedFromTemplate */ false,
		bpSchema.Metadata,
		bpSchema,
		params,
		l.funcRegistry,
		refChainCollector,
		l.resourceRegistry,
	)
}

func (l *defaultLoader) validateCustomVariableType(
	ctx context.Context,
	varName string,
	varSchema *schema.Variable,
	variables *schema.VariableMap,
	params bpcore.BlueprintParams,
) ([]*bpcore.Diagnostic, error) {
	providerCustomVarType, err := l.deriveProviderCustomVarType(ctx, varName, varSchema)
	if err != nil {
		return []*bpcore.Diagnostic{}, err
	}

	if providerCustomVarType == nil {
		line, col := source.PositionFromSourceMeta(varSchema.SourceMeta)
		return []*bpcore.Diagnostic{}, errInvalidCustomVariableType(
			varName, varSchema.Type.Value, line, col,
		)
	}

	return validation.ValidateCustomVariable(
		ctx,
		varName,
		varSchema,
		variables,
		params,
		providerCustomVarType,
		l.validateRuntimeValues,
	)
}

func (l *defaultLoader) deriveProviderCustomVarType(ctx context.Context, varName string, varSchema *schema.Variable) (provider.CustomVariableType, error) {
	// The provider should be keyed exactly by \w+ which is the custom type prefix. (e.g. "aws" in "aws/ec2/instanceType")
	// Avoid using a regular expression as it is more efficient to split the string.
	parts := strings.Split(string(varSchema.Type.Value), "/")
	if len(parts) == 0 {
		line, col := source.PositionFromSourceMeta(varSchema.SourceMeta)
		return nil, errInvalidCustomVariableType(varName, varSchema.Type.Value, line, col)
	}

	providerKey := parts[0]

	provider, ok := l.providers[providerKey]
	if !ok {
		line, col := source.PositionFromSourceMeta(varSchema.SourceMeta)
		return nil, errMissingProviderForCustomVarType(providerKey, varName, varSchema.Type.Value, line, col)
	}

	customVarType, err := provider.CustomVariableType(ctx, string(varSchema.Type.Value))
	if err != nil {
		return nil, err
	}

	return customVarType, nil
}

func (l *defaultLoader) validateDataSources(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.DataSources == nil {
		return diagnostics, nil
	}
	// To be as useful as possible, we'll collect and
	// report issues for all the problematic resources.
	dataSourceErrors := map[string][]error{}
	for name, dataSourceSchema := range bpSchema.DataSources.Values {
		currentDataSourceErrs := l.validateDataSource(
			ctx, &diagnostics, name, dataSourceSchema, bpSchema, params, refChainCollector,
		)
		if len(currentDataSourceErrs) > 0 {
			dataSourceErrors[name] = currentDataSourceErrs
		}
	}

	if len(dataSourceErrors) > 0 {
		return diagnostics, errDataSourceValidationError(dataSourceErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateDataSource(
	ctx context.Context,
	diagnostics *[]*bpcore.Diagnostic,
	name string,
	dataSourceSchema *schema.DataSource,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) []error {
	currentDataSourceErrs := []error{}
	err := validation.ValidateDataSourceName(name, bpSchema.DataSources)
	if err != nil {
		currentDataSourceErrs = append(currentDataSourceErrs, err)
	}

	var validateDataSourceDiagnostics []*bpcore.Diagnostic
	validateDataSourceDiagnostics, err = validation.ValidateDataSource(
		ctx,
		name,
		dataSourceSchema,
		bpSchema.DataSources,
		bpSchema,
		params,
		l.funcRegistry,
		refChainCollector,
		l.resourceRegistry,
		l.dataSourceRegistry,
	)
	*diagnostics = append(*diagnostics, validateDataSourceDiagnostics...)
	if err != nil {
		currentDataSourceErrs = append(currentDataSourceErrs, err)
	}

	refChainCollector.Collect(bpcore.DataSourceElementID(name), dataSourceSchema, "", []string{})
	return currentDataSourceErrs
}

func (l *defaultLoader) validateResources(
	ctx context.Context,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if bpSchema.Resources == nil {
		return diagnostics, nil
	}
	// To be as useful as possible, we'll collect and
	// report issues for all the problematic resources.
	resourceErrors := map[string][]error{}
	for name, resourceSchema := range bpSchema.Resources.Values {
		currentResouceErrs := l.validateResource(
			ctx, &diagnostics, name, resourceSchema, bpSchema, params, refChainCollector,
		)
		if len(currentResouceErrs) > 0 {
			resourceErrors[name] = currentResouceErrs
		}
	}

	if len(resourceErrors) > 0 {
		return diagnostics, errResourceValidationError(resourceErrors)
	}

	return diagnostics, nil
}

func (l *defaultLoader) validateResource(
	ctx context.Context,
	diagnostics *[]*bpcore.Diagnostic,
	name string,
	resourceSchema *schema.Resource,
	bpSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
	refChainCollector validation.RefChainCollector,
) []error {
	currentResouceErrs := []error{}
	err := validation.ValidateResourceName(name, bpSchema.Resources)
	if err != nil {
		currentResouceErrs = append(currentResouceErrs, err)
	}
	err = validation.PreValidateResourceSpec(ctx, name, resourceSchema, bpSchema.Resources)
	if err != nil {
		currentResouceErrs = append(currentResouceErrs, err)
	}

	var validateResourceDiagnostics []*bpcore.Diagnostic
	validateResourceDiagnostics, err = validation.ValidateResource(
		ctx,
		name,
		resourceSchema,
		bpSchema.Resources,
		bpSchema,
		params,
		l.funcRegistry,
		refChainCollector,
		l.resourceRegistry,
		slices.Contains(l.derivedFromTemplates, name),
	)
	*diagnostics = append(*diagnostics, validateResourceDiagnostics...)
	if err != nil {
		currentResouceErrs = append(currentResouceErrs, err)
	}

	refChainCollector.Collect(bpcore.ResourceElementID(name), resourceSchema, "", []string{})
	return currentResouceErrs
}

func (l *defaultLoader) LoadString(
	ctx context.Context,
	blueprintSpec string,
	inputFormat schema.SpecFormat,
	params bpcore.BlueprintParams,
) (BlueprintContainer, error) {
	loadInfo := &loadBlueprintInfo{
		specOrFilePath: blueprintSpec,
	}
	container, _, err := l.loadSpecAndLinkInfo(ctx, loadInfo, params, schema.LoadString, predefinedFormatFactory(inputFormat))
	return container, err
}

func (l *defaultLoader) ValidateString(
	ctx context.Context,
	blueprintSpec string,
	inputFormat schema.SpecFormat,
	params bpcore.BlueprintParams,
) (*ValidationResult, error) {
	loadInfo := &loadBlueprintInfo{
		specOrFilePath: blueprintSpec,
	}
	container, diagnostics, err := l.loadSpecAndLinkInfo(ctx, loadInfo, params, schema.LoadString, predefinedFormatFactory(inputFormat))
	if err != nil {
		return &ValidationResult{
			Diagnostics: diagnostics,
			Schema:      getSchemaFromContainer(container),
		}, err
	}

	return &ValidationResult{
		Diagnostics: diagnostics,
		Schema:      getSchemaFromContainer(container),
		LinkInfo:    container.SpecLinkInfo(),
	}, nil
}

func (l *defaultLoader) LoadFromSchema(
	ctx context.Context,
	blueprintSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
) (BlueprintContainer, error) {
	loadInfo := &loadBlueprintInfo{
		preloadedSchema: blueprintSchema,
	}
	container, _, err := l.loadSpecAndLinkInfo(
		ctx,
		loadInfo,
		params,
		/* schemaLoader */ nil,
		/* formatLoader */ nil,
	)
	return container, err
}

func (l *defaultLoader) ValidateFromSchema(
	ctx context.Context,
	blueprintSchema *schema.Blueprint,
	params bpcore.BlueprintParams,
) (*ValidationResult, error) {
	loadInfo := &loadBlueprintInfo{
		preloadedSchema: blueprintSchema,
	}
	container, diagnostics, err := l.loadSpecAndLinkInfo(
		ctx,
		loadInfo,
		params,
		/* schemaLoader */ nil,
		/* formatLoader */ nil,
	)
	if err != nil {
		return &ValidationResult{
			Diagnostics: diagnostics,
			Schema:      blueprintSchema,
		}, err
	}
	return &ValidationResult{
		Diagnostics: diagnostics,
		Schema:      container.BlueprintSpec().Schema(),
		LinkInfo:    container.SpecLinkInfo(),
	}, nil
}

func loadBlueprintSpec(
	loadInfo *loadBlueprintInfo,
	formatLoader func(string) (schema.SpecFormat, error),
	loader schema.Loader,
) (*schema.Blueprint, error) {
	if loadInfo.preloadedSchema != nil {
		return loadInfo.preloadedSchema, nil
	}

	format, err := formatLoader(loadInfo.specOrFilePath)
	if err != nil {
		return nil, err
	}

	return loader(loadInfo.specOrFilePath, format)
}

func getSchemaFromContainer(
	container BlueprintContainer,
) *schema.Blueprint {
	schema := (*schema.Blueprint)(nil)
	if container != nil {
		spec := container.BlueprintSpec()
		if spec != nil {
			schema = spec.Schema()
		}
	}
	return schema
}
