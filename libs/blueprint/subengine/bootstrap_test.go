package subengine

import (
	"os"

	"github.com/stretchr/testify/suite"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/internal"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/providerhelpers"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/transform"
)

func newTestAWSProvider() provider.Provider {
	return &internal.ProviderMock{
		NamespaceValue: "aws",
		Resources: map[string]provider.Resource{
			"aws/dynamodb/table":  &internal.DynamoDBTableResource{},
			"aws/lambda/function": &internal.LambdaFunctionResource{},
		},
		Links: map[string]provider.Link{},
		CustomVariableTypes: map[string]provider.CustomVariableType{
			"aws/ec2/instanceType": &internal.InstanceTypeCustomVariableType{},
		},
		DataSources: map[string]provider.DataSource{
			"aws/vpc": &internal.VPCDataSource{},
		},
	}
}

type SubResolverTestContainer struct {
	specFixtureFiles               map[string]string
	specFixtureSchemas             map[string]*schema.Blueprint
	resourceRegistry               resourcehelpers.Registry
	funcRegistry                   provider.FunctionRegistry
	dataSourceRegistry             provider.DataSourceRegistry
	stateContainer                 state.Container
	resourceCache                  *core.Cache[*provider.ResolvedResource]
	resourceTemplateInputElemCache *core.Cache[[]*core.MappingNode]
	childExportFieldCache          *core.Cache[*ChildExportFieldInfo]
}

func (s *SubResolverTestContainer) populateSpecFixtureSchemas(
	fileMap map[string]string,
	suite *suite.Suite,
) {
	s.specFixtureFiles = fileMap
	s.specFixtureSchemas = make(map[string]*schema.Blueprint)

	for name, filePath := range s.specFixtureFiles {
		specBytes, err := os.ReadFile(filePath)
		if err != nil {
			suite.FailNow(err.Error())
		}
		blueprintStr := string(specBytes)
		blueprint, err := schema.LoadString(blueprintStr, schema.YAMLSpecFormat)
		if err != nil {
			suite.FailNow(err.Error())
		}
		s.specFixtureSchemas[name] = blueprint
	}
}

func (s *SubResolverTestContainer) populateDependencies() {
	s.stateContainer = internal.NewMemoryStateContainer()
	providers := map[string]provider.Provider{
		"aws": newTestAWSProvider(),
		"core": providerhelpers.NewCoreProvider(
			s.stateContainer.Links(),
			core.BlueprintInstanceIDFromContext,
			os.Getwd,
			core.SystemClock{},
		),
	}
	s.funcRegistry = provider.NewFunctionRegistry(providers)
	s.resourceRegistry = resourcehelpers.NewRegistry(
		providers,
		map[string]transform.SpecTransformer{},
	)
	s.dataSourceRegistry = provider.NewDataSourceRegistry(
		providers,
	)
	s.resourceCache = core.NewCache[*provider.ResolvedResource]()
	s.resourceTemplateInputElemCache = core.NewCache[[]*core.MappingNode]()
	s.childExportFieldCache = core.NewCache[*ChildExportFieldInfo]()
}

func convertToTemplateResourceInstance(
	resource *schema.Resource,
) *schema.Resource {
	// Exclude `each` property as per the template expansion process
	// in the change staging process.
	return &schema.Resource{
		Type:         resource.Type,
		Description:  resource.Description,
		Metadata:     resource.Metadata,
		DependsOn:    resource.DependsOn,
		Condition:    resource.Condition,
		LinkSelector: resource.LinkSelector,
		Spec:         resource.Spec,
		SourceMeta:   resource.SourceMeta,
	}
}
