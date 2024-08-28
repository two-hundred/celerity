package corefunctions

import (
	"context"
	"fmt"

	"github.com/two-hundred/celerity/libs/blueprint/function"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

// LinkFunction provides the implementation of the
// core "link" function defined in the blueprint specification.
type LinkFunction struct {
	definition          *function.Definition
	linkStateRetriever  LinkStateRetriever
	blueprintInstanceID string
}

// NewLinkFunction creates a new instance of the LinkFunction with
// a complete function definition.
// This function takes a state container for the current blueprint execution
// environment as a parameter to allow the link function to retrieve
// and return the state of a link between two resources.
// This also requires the instance ID for the current blueprint.
func NewLinkFunction(linkStateRetriever LinkStateRetriever, blueprintInstanceID string) provider.Function {
	return &LinkFunction{
		linkStateRetriever:  linkStateRetriever,
		blueprintInstanceID: blueprintInstanceID,
		definition: &function.Definition{
			Description: "A function to retrieve the state of a link between two resources.",
			FormattedDescription: "A function to retrieve the state of a link between two resources.\n\n" +
				"**Examples:**\n\n" +
				"Using resource references:\n" +
				"```${link(resources.orderApi, resources.createOrderFunction)}```\n" +
				"Using implicit resource references (identifiers without a namespace are either resources or functions):\n" +
				"```${link(orderApi, listOrdersFunction)}```\n" +
				"Using string literals:\n" +
				"```${link(\"orderApi\", \"deleteOrderFunction\")}```",
			Parameters: []function.Parameter{
				&function.AnyParameter{
					UnionTypes: []function.ValueTypeDefinition{
						&function.ValueTypeDefinitionScalar{
							Type: function.ValueTypeString,
						},
						&function.ValueTypeDefinitionObject{
							Label: "ResourceRef",
							AttributeTypes: map[string]function.AttributeType{
								"name": {
									Type: &function.ValueTypeDefinitionScalar{
										Type: function.ValueTypeString,
									},
								},
							},
						},
					},
					Description: "Resource A in the relationship.",
				},
				&function.AnyParameter{
					UnionTypes: []function.ValueTypeDefinition{
						&function.ValueTypeDefinitionScalar{
							Type: function.ValueTypeString,
						},
						&function.ValueTypeDefinitionObject{
							Label: "ResourceRef",
							AttributeTypes: map[string]function.AttributeType{
								"name": {
									Type: &function.ValueTypeDefinitionScalar{
										Type: function.ValueTypeString,
									},
								},
							},
						},
					},
					Description: "Resource A in the relationship.",
				},
			},
			Return: &function.ObjectReturn{
				Description: "An object containing all the information about the link between " +
					"the two resources made available by the provider that powers the link.",
			},
		},
	}
}

func (f *LinkFunction) GetDefinition(
	ctx context.Context,
	input *provider.FunctionGetDefinitionInput,
) (*provider.FunctionGetDefinitionOutput, error) {
	return &provider.FunctionGetDefinitionOutput{
		Definition: f.definition,
	}, nil
}

func (f *LinkFunction) Call(
	ctx context.Context,
	input *provider.FunctionCallInput,
) (*provider.FunctionCallOutput, error) {
	var resourceA interface{}
	var resourceB interface{}
	err := input.Arguments.GetMultipleVars(ctx, &resourceA, &resourceB)
	if err != nil {
		return nil, err
	}

	resourceAStr, err := extractResourceName(input, 0, resourceA)
	if err != nil {
		return nil, err
	}

	resourceBStr, err := extractResourceName(input, 1, resourceB)
	if err != nil {
		return nil, err
	}

	linkState, err := f.linkStateRetriever.GetLink(
		ctx,
		f.blueprintInstanceID,
		fmt.Sprintf("%s::%s", resourceAStr, resourceBStr),
	)

	if err != nil {
		return nil, function.NewFuncCallError(
			fmt.Sprintf(
				"failed to retrieve link state for %q and %q: %v",
				resourceAStr,
				resourceBStr,
				err,
			),
			function.FuncCallErrorCodeFunctionCall,
			input.CallContext.CallStackSnapshot(),
		)
	}

	linkStateMap := linkStateToInterfaceMap(linkState)

	return &provider.FunctionCallOutput{
		ResponseData: linkStateMap,
	}, nil
}

func extractResourceName(input *provider.FunctionCallInput, index int, resourceArg interface{}) (string, error) {
	resourceStr, isResourceStr := resourceArg.(string)
	if !isResourceStr {
		resourceName, isMapAttr := resourceArg.(map[string]interface{})["name"].(string)
		if !isMapAttr {
			return "", function.NewFuncCallError(
				fmt.Sprintf("argument %d must be a string or a resource reference.", index),
				function.FuncCallErrorCodeInvalidInput,
				input.CallContext.CallStackSnapshot(),
			)
		}
		return resourceName, nil
	}

	return resourceStr, nil
}

// Converts a link state struct to a map of string to interface
// to be compatible with any function that the value can be passed
// to.
func linkStateToInterfaceMap(linkState state.LinkState) map[string]interface{} {
	intermediaryResourceStates := make([]interface{}, 0, len(linkState.IntermediaryResourceStates))
	for _, state := range linkState.IntermediaryResourceStates {
		intermediaryResourceStates = append(intermediaryResourceStates, resourceStateToInterfaceMap(state))
	}

	return map[string]interface{}{
		"intermediaryResourceStates": intermediaryResourceStates,
		"linkData":                   linkState.LinkData,
	}
}

func resourceStateToInterfaceMap(resourceState *state.ResourceState) map[string]interface{} {
	failureReasons := make([]interface{}, 0, len(resourceState.FailureReasons))
	for _, reason := range resourceState.FailureReasons {
		failureReasons = append(failureReasons, reason)
	}

	return map[string]interface{}{
		"resourceID":     resourceState.ResourceID,
		"status":         resourceState.Status,
		"resourceData":   resourceState.ResourceData,
		"failureReasons": failureReasons,
	}
}

// LinkStateRetriever is an interface that defines a service
// that can be used to retrieve the state of a link between
// two resources in a blueprint instance.
type LinkStateRetriever interface {
	// GetLink deals with retrieving the state for a given link
	// in the provided blueprint instance.
	GetLink(ctx context.Context, instanceID string, linkID string) (state.LinkState, error)
}