package subengine

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/errors"
	"github.com/two-hundred/celerity/libs/blueprint/internal"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/providerhelpers"
	"github.com/two-hundred/celerity/libs/blueprint/resourcehelpers"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/blueprint/transform"
)

type SubstitutionResourceEachResolverTestSuite struct {
	specFixtureFiles   map[string]string
	specFixtureSchemas map[string]*schema.Blueprint
	resourceRegistry   resourcehelpers.Registry
	funcRegistry       provider.FunctionRegistry
	dataSourceRegistry provider.DataSourceRegistry
	stateContainer     state.Container
	resourceCache      *core.Cache[*provider.ResolvedResource]
	suite.Suite
}

const (
	resolveInResourceEachFixtureName      = "resolve-resource-each"
	resolveInResourceEachFail1FixtureName = "resolve-resource-each-fail-1"
	resolveInResourceEachFail2FixtureName = "resolve-resource-each-fail-2"
	resolveInResourceEachFail3FixtureName = "resolve-resource-each-fail-3"
)

func (s *SubstitutionResourceEachResolverTestSuite) SetupSuite() {
	s.specFixtureFiles = map[string]string{
		resolveInResourceEachFixtureName:      "__testdata/sub-resolver/resolve-resource-each-blueprint.yml",
		resolveInResourceEachFail1FixtureName: "__testdata/sub-resolver/resolve-resource-each-fail-1-blueprint.yml",
		resolveInResourceEachFail2FixtureName: "__testdata/sub-resolver/resolve-resource-each-fail-2-blueprint.yml",
		resolveInResourceEachFail3FixtureName: "__testdata/sub-resolver/resolve-resource-each-fail-3-blueprint.yml",
	}
	s.specFixtureSchemas = make(map[string]*schema.Blueprint)

	for name, filePath := range s.specFixtureFiles {
		specBytes, err := os.ReadFile(filePath)
		if err != nil {
			s.FailNow(err.Error())
		}
		blueprintStr := string(specBytes)
		blueprint, err := schema.LoadString(blueprintStr, schema.YAMLSpecFormat)
		if err != nil {
			s.FailNow(err.Error())
		}
		s.specFixtureSchemas[name] = blueprint
	}
}

func (s *SubstitutionResourceEachResolverTestSuite) SetupTest() {
	s.stateContainer = internal.NewMemoryStateContainer()
	providers := map[string]provider.Provider{
		"aws": newTestAWSProvider(),
		"core": providerhelpers.NewCoreProvider(
			s.stateContainer,
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
}

func (s *SubstitutionResourceEachResolverTestSuite) Test_resolves_substitutions_in_resource_each_for_change_staging() {
	blueprint := s.specFixtureSchemas[resolveInResourceEachFixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveResourceEachTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)

	result, err := subResolver.ResolveResourceEach(
		context.TODO(),
		"ordersTable",
		blueprint.Resources.Values["ordersTable"],
		ResolveForChangeStaging,
	)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	region1 := "us-west-2"
	region2 := "us-east-1"
	region3 := "eu-west-2"
	s.Assert().Equal(
		[]*core.MappingNode{
			{
				Literal: &core.ScalarValue{
					StringValue: &region1,
				},
			},
			{
				Literal: &core.ScalarValue{
					StringValue: &region2,
				},
			},
			{
				Literal: &core.ScalarValue{
					StringValue: &region3,
				},
			},
		},
		result,
	)
}

func (s *SubstitutionResourceEachResolverTestSuite) Test_fails_when_resource_each_depends_on_resource() {
	blueprint := s.specFixtureSchemas[resolveInResourceEachFail1FixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveResourceEachTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)

	result, err := subResolver.ResolveResourceEach(
		context.TODO(),
		"ordersTable",
		blueprint.Resources.Values["ordersTable"],
		ResolveForChangeStaging,
	)
	s.Assert().Error(err)
	s.Assert().Nil(result)
	runErr, isRunErr := err.(*errors.RunError)
	s.Assert().True(isRunErr)
	s.Assert().Equal(ErrorReasonCodeDisallowedElementType, runErr.ReasonCode)
	s.Assert().Equal(
		"run error: [resources.ordersTable]: element type \"resource\" can not be a dependency for the \"each\" property, "+
			"a dependency can be either a direct or indirect reference to an element in a blueprint, "+
			"be sure to check the full trail of references",
		runErr.Error(),
	)
}

func (s *SubstitutionResourceEachResolverTestSuite) Test_fails_when_resource_each_depends_on_child_blueprint() {
	blueprint := s.specFixtureSchemas[resolveInResourceEachFail2FixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveResourceEachTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)

	result, err := subResolver.ResolveResourceEach(
		context.TODO(),
		"ordersTable",
		blueprint.Resources.Values["ordersTable"],
		ResolveForChangeStaging,
	)
	s.Assert().Error(err)
	s.Assert().Nil(result)
	runErr, isRunErr := err.(*errors.RunError)
	s.Assert().True(isRunErr)
	s.Assert().Equal(ErrorReasonCodeDisallowedElementType, runErr.ReasonCode)
	s.Assert().Equal(
		"run error: [resources.ordersTable]: element type \"child\" can not be a dependency for the \"each\" property, "+
			"a dependency can be either a direct or indirect reference to an element in a blueprint, "+
			"be sure to check the full trail of references",
		runErr.Error(),
	)
}

func (s *SubstitutionResourceEachResolverTestSuite) Test_fails_when_resource_each_resolves_to_a_value_that_is_not_a_list() {
	blueprint := s.specFixtureSchemas[resolveInResourceEachFail3FixtureName]
	spec := internal.NewBlueprintSpecMock(blueprint)
	params := resolveResourceEachTestParams()
	subResolver := NewDefaultSubstitutionResolver(
		s.funcRegistry,
		s.resourceRegistry,
		s.dataSourceRegistry,
		s.stateContainer,
		s.resourceCache,
		spec,
		params,
	)

	result, err := subResolver.ResolveResourceEach(
		context.TODO(),
		"ordersTable",
		blueprint.Resources.Values["ordersTable"],
		ResolveForChangeStaging,
	)
	s.Assert().Error(err)
	s.Assert().Nil(result)
	runErr, isRunErr := err.(*errors.RunError)
	s.Assert().True(isRunErr)
	s.Assert().Equal(ErrorReasonCodeResourceEachInvalidType, runErr.ReasonCode)
	s.Assert().Equal(
		"run error: [resources.ordersTable]: `each` property in resource template"+
			" \"ordersTable\" must yield an array, string found",
		runErr.Error(),
	)
}

func resolveResourceEachTestParams() *internal.Params {
	environment := "production-env"
	enableOrderTableTrigger := true
	region := "us-west-2"
	deployOrdersTableToRegions := "[\"us-west-2\",\"us-east-1\",\"eu-west-2\"]"
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
	}
	return internal.NewParams(
		map[string]map[string]*core.ScalarValue{},
		map[string]*core.ScalarValue{},
		blueprintVars,
	)
}

func TestSubstitutionResourceEachResolverTestSuite(t *testing.T) {
	suite.Run(t, new(SubstitutionResourceEachResolverTestSuite))
}