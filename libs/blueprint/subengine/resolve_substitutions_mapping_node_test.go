package subengine

import (
	"context"
	"testing"

	"github.com/bradleyjkemp/cupaloy"
	"github.com/stretchr/testify/suite"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/internal"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type SubstitutionMappingNodeResolverTestSuite struct {
	SubResolverTestContainer
	suite.Suite
}

const (
	resolveInMappingNodeFixtureName = "resolve-in-mapping-node"
)

func (s *SubstitutionMappingNodeResolverTestSuite) SetupSuite() {
	s.populateSpecFixtureSchemas(
		map[string]string{
			resolveInMappingNodeFixtureName: "__testdata/sub-resolver/resolve-in-mapping-node-blueprint.yml",
		},
		&s.Suite,
	)
}

func (s *SubstitutionMappingNodeResolverTestSuite) SetupTest() {
	s.populateDependencies()
}

func (s *SubstitutionMappingNodeResolverTestSuite) Test_resolves_substitutions_in_mapping_node_for_change_staging() {
	blueprint := s.specFixtureSchemas[resolveInMappingNodeFixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveInMappingNodeTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)

	result, err := subResolver.ResolveInMappingNode(
		context.TODO(),
		"metadata",
		blueprint.Metadata,
		&ResolveMappingNodeTargetInfo{
			ResolveFor: ResolveForChangeStaging,
		},
	)
	s.Require().NoError(err)
	s.Require().NotNil(result)

	err = cupaloy.Snapshot(result)
	s.Require().NoError(err)
}

func (s *SubstitutionMappingNodeResolverTestSuite) Test_resolves_substitutions_in_mapping_node_for_deployment() {
	blueprint := s.specFixtureSchemas[resolveInMappingNodeFixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveInMappingNodeTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)
	// ordersTable.spec.id is used in the referenced data source and should be resolved using the resource
	// state.
	err := s.stateContainer.SaveInstance(context.Background(), state.InstanceState{
		InstanceID: testInstanceID,
	})
	s.Require().NoError(err)

	resourceID := "test-orders-table-309428320"
	err = s.stateContainer.SaveResource(
		context.Background(),
		testInstanceID,
		state.ResourceState{
			ResourceID:   resourceID,
			ResourceName: "ordersTable",
			ResourceSpecData: &core.MappingNode{
				Fields: map[string]*core.MappingNode{
					"id": {
						Literal: &core.ScalarValue{
							StringValue: &resourceID,
						},
					},
				},
			},
		},
	)
	s.Require().NoError(err)

	// Make sure the current instance ID can be retrieved from the context when fetching
	// state from the state container to resolve the resource spec id reference.
	ctx := context.WithValue(
		context.Background(),
		core.BlueprintInstanceIDKey,
		testInstanceID,
	)

	// coreInfra.region is used in the metadata and should be resolved using the child blueprint
	// state.
	childBlueprintRegion := "eu-west-1"
	err = s.stateContainer.SaveChild(
		context.Background(),
		testInstanceID,
		"coreInfra",
		state.InstanceState{
			Exports: map[string]*core.MappingNode{
				"region": {
					Literal: &core.ScalarValue{
						StringValue: &childBlueprintRegion,
					},
				},
			},
		},
	)
	s.Require().NoError(err)

	// The resource must be resolved before the data source can be resolved.
	// During change staging, the blueprint container will make sure that resources
	// are ordered correctly so that by the time any resource is referenced, it has
	// already been resolved.
	s.resourceCache.Set("ordersTable", &provider.ResolvedResource{})

	result, err := subResolver.ResolveInMappingNode(
		ctx,
		"metadata",
		blueprint.Metadata,
		&ResolveMappingNodeTargetInfo{
			ResolveFor:        ResolveForDeployment,
			PartiallyResolved: partiallyResolvedMappingNode(),
		},
	)
	s.Require().NoError(err)
	s.Require().NotNil(result)

	err = cupaloy.Snapshot(result)
	s.Require().NoError(err)
}

func partiallyResolvedMappingNode() *core.MappingNode {
	build := "esbuild"
	return &core.MappingNode{
		Fields: map[string]*core.MappingNode{
			"build": {
				Literal: &core.ScalarValue{
					StringValue: &build,
				},
			},
			"networkingSummary": {
				Fields: map[string]*core.MappingNode{
					"coreInfraRegion": (*core.MappingNode)(nil),
					"vpc":             (*core.MappingNode)(nil),
				},
			},
		},
	}
}

func resolveInMappingNodeTestParams() *internal.Params {
	environment := "production-env"
	enableOrderTableTrigger := true
	region := "us-west-2"
	deployOrdersTableToRegions := "[\"us-west-2\",\"us-east-1\"]"
	relatedInfo := "[{\"id\":\"test-info-1\"},{\"id\":\"test-info-2\"}]"
	blueprintVars := map[string]*core.ScalarValue{
		"environment": {
			StringValue: &environment,
		},
		"region": {
			StringValue: &region,
		},
		"deployOrdersTableToRegions": {
			StringValue: &deployOrdersTableToRegions,
		},
		"enableOrderTableTrigger": {
			BoolValue: &enableOrderTableTrigger,
		},
		"relatedInfo": {
			StringValue: &relatedInfo,
		},
	}
	return internal.NewParams(
		map[string]map[string]*core.ScalarValue{},
		map[string]*core.ScalarValue{},
		blueprintVars,
	)
}

func TestSubstitutionMappingNodeResolverTestSuite(t *testing.T) {
	suite.Run(t, new(SubstitutionMappingNodeResolverTestSuite))
}