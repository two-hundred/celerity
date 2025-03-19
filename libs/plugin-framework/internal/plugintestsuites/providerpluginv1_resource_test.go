package plugintestsuites

import (
	"context"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/substitutions"
	"github.com/two-hundred/celerity/libs/plugin-framework/errorsv1"
	"github.com/two-hundred/celerity/libs/plugin-framework/internal/testprovider"
	"github.com/two-hundred/celerity/libs/plugin-framework/internal/testutils"
)

const (
	lambdaFunctionResourceType = "aws/lambda/function"
)

func (s *ProviderPluginV1Suite) Test_custom_validate_resource() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.CustomValidate(
		context.Background(),
		resourceValidateInput(),
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		testprovider.ResourceLambdaFunctionValidateOutput(),
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_custom_validate_resource_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.CustomValidate(
		context.Background(),
		resourceValidateInput(),
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderCustomValidateResource,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_custom_validate_resource_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.CustomValidate(
		context.Background(),
		resourceValidateInput(),
	)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "internal error occurred applying custom validation for resource")
}

func resourceValidateInput() *provider.ResourceValidateInput {
	return &provider.ResourceValidateInput{
		SchemaResource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{
				Value: "aws/lambda/function",
			},
			Metadata: &schema.Metadata{
				Annotations: &schema.StringOrSubstitutionsMap{
					Values: map[string]*substitutions.StringOrSubstitutions{},
				},
				Labels: &schema.StringMap{
					Values: map[string]string{},
				},
			},
			Spec: &core.MappingNode{
				Fields: map[string]*core.MappingNode{
					"functionName": core.MappingNodeFromString("my-function"),
				},
			},
		},
		ProviderContext: testutils.CreateTestProviderContext("aws"),
	}
}

func (s *ProviderPluginV1Suite) Test_get_resource_spec_definition() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.GetSpecDefinition(
		context.Background(),
		&provider.ResourceGetSpecDefinitionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		&provider.ResourceGetSpecDefinitionOutput{
			SpecDefinition: &provider.ResourceSpecDefinition{
				Schema:  testprovider.ResourceLambdaFunctionSchema(),
				IDField: "arn",
			},
		},
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_spec_definition_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.GetSpecDefinition(
		context.Background(),
		&provider.ResourceGetSpecDefinitionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderGetResourceSpecDefinition,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_spec_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.GetSpecDefinition(
		context.Background(),
		&provider.ResourceGetSpecDefinitionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "internal error occurred retrieving resource spec definition")
}

func (s *ProviderPluginV1Suite) Test_resource_can_link_to() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.CanLinkTo(
		context.Background(),
		&provider.ResourceCanLinkToInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		&provider.ResourceCanLinkToOutput{
			CanLinkTo: []string{"aws/dynamodb/table"},
		},
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_resource_can_link_to_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(
		context.Background(),
		lambdaFunctionResourceType,
	)
	s.Require().NoError(err)

	_, err = resource.CanLinkTo(
		context.Background(),
		&provider.ResourceCanLinkToInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderCheckCanResourceLinkTo,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_resource_can_link_to_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.CanLinkTo(
		context.Background(),
		&provider.ResourceCanLinkToInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(
		err.Error(),
		"internal error occurred retrieving the resources that can be linked to",
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_stabilised_dependencies() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.GetStabilisedDependencies(
		context.Background(),
		&provider.ResourceStabilisedDependenciesInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		&provider.ResourceStabilisedDependenciesOutput{
			StabilisedDependencies: []string{"aws/sqs/queue"},
		},
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_stabilised_deps_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(
		context.Background(),
		lambdaFunctionResourceType,
	)
	s.Require().NoError(err)

	_, err = resource.GetStabilisedDependencies(
		context.Background(),
		&provider.ResourceStabilisedDependenciesInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderGetResourceStabilisedDeps,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_stabilised_deps_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.GetStabilisedDependencies(
		context.Background(),
		&provider.ResourceStabilisedDependenciesInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(
		err.Error(),
		"internal error occurred retrieving the stabilised dependencies for a resource",
	)
}

func (s *ProviderPluginV1Suite) Test_check_is_resource_common_terminal() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.IsCommonTerminal(
		context.Background(),
		&provider.ResourceIsCommonTerminalInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		&provider.ResourceIsCommonTerminalOutput{
			IsCommonTerminal: false,
		},
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_check_is_resource_common_terminal_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(
		context.Background(),
		lambdaFunctionResourceType,
	)
	s.Require().NoError(err)

	_, err = resource.IsCommonTerminal(
		context.Background(),
		&provider.ResourceIsCommonTerminalInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderCheckIsResourceCommonTerminal,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_check_is_resource_common_terminal_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.IsCommonTerminal(
		context.Background(),
		&provider.ResourceIsCommonTerminalInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(
		err.Error(),
		"internal error occurred retrieving whether or not the resource is a common terminal",
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.GetType(
		context.Background(),
		&provider.ResourceGetTypeInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	s.Assert().Equal(
		&provider.ResourceGetTypeOutput{
			Type:  "aws/lambda/function",
			Label: "AWS Lambda Function",
		},
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(
		context.Background(),
		lambdaFunctionResourceType,
	)
	s.Require().NoError(err)

	_, err = resource.GetType(
		context.Background(),
		&provider.ResourceGetTypeInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderGetResourceType,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.GetType(
		context.Background(),
		&provider.ResourceGetTypeInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(
		err.Error(),
		"internal error occurred retrieving resource type information",
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type_description() {
	resource, err := s.provider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	output, err := resource.GetTypeDescription(
		context.Background(),
		&provider.ResourceGetTypeDescriptionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Require().NoError(err)
	expected := testprovider.ResourceLambdaFunctionTypeDescription()
	s.Assert().Equal(
		expected,
		output,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type_description_fails_for_unexpected_host() {
	resource, err := s.providerWrongHost.Resource(
		context.Background(),
		lambdaFunctionResourceType,
	)
	s.Require().NoError(err)

	_, err = resource.GetTypeDescription(
		context.Background(),
		&provider.ResourceGetTypeDescriptionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	testutils.AssertInvalidHost(
		err,
		errorsv1.PluginActionProviderGetResourceTypeDescription,
		testWrongHostID,
		&s.Suite,
	)
}

func (s *ProviderPluginV1Suite) Test_get_resource_type_description_reports_expected_error_for_failure() {
	resource, err := s.failingProvider.Resource(context.Background(), lambdaFunctionResourceType)
	s.Require().NoError(err)

	_, err = resource.GetTypeDescription(
		context.Background(),
		&provider.ResourceGetTypeDescriptionInput{
			ProviderContext: testutils.CreateTestProviderContext("aws"),
		},
	)
	s.Assert().Error(err)
	s.Assert().Contains(
		err.Error(),
		"internal error occurred retrieving resource type description",
	)
}
