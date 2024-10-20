package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/common/core"
)

type OrderingTestSuite struct {
	orderFixtures []orderChainLinkFixture
	suite.Suite
}

type orderChainLinkFixture struct {
	inputChains []*links.ChainLink
	// All the resource names that are expected to be present
	// in the ordered flattened list of links (resources)
	// to be deployed.
	expectedPresent []string
	// A two-dimensional slice of resources with hard links that must
	// come in the provided order, it doesn't matter what the exact
	// positions in the list they are as long as they are in the given order.
	orderedExpected [][]string
}

func (s *OrderingTestSuite) SetUpSuite() {
	s.orderFixtures = []orderChainLinkFixture{
		orderFixture1,
	}
}

func (s *OrderingTestSuite) Test_order_links_for_deployment_with_circular_links() {
	orderedLinks, err := OrderLinksForDeployment(context.TODO(), orderFixture1.inputChains, nil)
	s.Assert().NoError(err)
	s.Assert().Len(orderedLinks, len(orderFixture1.expectedPresent))
	s.Assert().Len(
		core.Filter(
			orderedLinks,
			inExpected(orderFixture1.expectedPresent),
		),
		len(orderFixture1.expectedPresent),
	)

	for _, orderedExpectedSet := range orderFixture1.orderedExpected {
		s.assertOrderedExpected(orderedLinks, orderedExpectedSet)
	}
}

func (s *OrderingTestSuite) Test_order_links_for_deployment_without_circular_links() {
	orderedLinks, err := OrderLinksForDeployment(context.TODO(), orderFixture2.inputChains, nil)
	s.Assert().NoError(err)
	s.Assert().Len(orderedLinks, len(orderFixture2.expectedPresent))
	s.Assert().Len(
		core.Filter(
			orderedLinks,
			inExpected(orderFixture2.expectedPresent),
		),
		len(orderFixture2.expectedPresent),
	)

	for _, orderedExpectedSet := range orderFixture2.orderedExpected {
		s.assertOrderedExpected(orderedLinks, orderedExpectedSet)
	}
}

func (s *OrderingTestSuite) assertOrderedExpected(actual []*links.ChainLink, orderedExpected []string) {
	expectedItemsInOrder := core.Filter(actual, inExpected(orderedExpected))
	inOrder := true
	i := 0
	var linkA *links.ChainLink
	var linkB *links.ChainLink

	for inOrder && i < len(expectedItemsInOrder) {
		if i+1 < len(expectedItemsInOrder) {
			linkA = expectedItemsInOrder[i]
			linkB = expectedItemsInOrder[i+1]
			inOrder = linkA.ResourceName == orderedExpected[i] && linkB.ResourceName == orderedExpected[i+1]
		}
		i += 2
	}

	if !inOrder {
		s.Failf("expected \"%s\" to come before \"%s\"", linkB.ResourceName, linkA.ResourceName)
	}
}

func inExpected(expectedResourceNames []string) func(*links.ChainLink, int) bool {
	return func(currentLink *links.ChainLink, index int) bool {
		return core.SliceContainsComparable(expectedResourceNames, currentLink.ResourceName)
	}
}

var testProviderImpl = newTestAWSProvider()

var orderFixture1 = orderChainLinkFixture{
	inputChains: orderFixture1Chains(),
	expectedPresent: []string{
		"orderApi",
		"getOrdersFunction",
		"createOrderFunction",
		"updateOrderFunction",
		"ordersTable",
		"ordersStream",
		"statsAccumulatorFunction",
		"secondaryOrdersDB",
	},
	orderedExpected: [][]string{{"ordersTable", "ordersStream"}},
}

func orderFixture1Chains() []*links.ChainLink {
	apiGatewayLambdaLinkImpl, _ := testProviderImpl.Link(context.TODO(), "aws/apigateway/api", "aws/lambda/function")
	orderApi := &links.ChainLink{
		ResourceName: "orderApi",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/apigateway/api"},
		},
		Paths: []string{},
		LinkImplementations: map[string]provider.Link{
			"getOrdersFunction":   apiGatewayLambdaLinkImpl,
			"createOrderFunction": apiGatewayLambdaLinkImpl,
			"updateOrderFunction": apiGatewayLambdaLinkImpl,
		},
		LinkedFrom: []*links.ChainLink{},
		LinksTo:    []*links.ChainLink{},
	}

	lambdaDynamoDBTableLink, _ := testProviderImpl.Link(context.TODO(), "aws/lambda/function", "aws/dynamodb/table")
	getOrdersFunction := &links.ChainLink{
		ResourceName: "getOrdersFunction",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/lambda/function"},
		},
		Paths: []string{"/orderApi"},
		LinkImplementations: map[string]provider.Link{
			"ordersTable": lambdaDynamoDBTableLink,
		},
		LinkedFrom: []*links.ChainLink{
			orderApi,
		},
	}
	createOrderFunction := &links.ChainLink{
		ResourceName: "createOrderFunction",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/lambda/function"},
		},
		LinkImplementations: map[string]provider.Link{
			"ordersTable": lambdaDynamoDBTableLink,
		},
		Paths: []string{"/orderApi"},
		LinkedFrom: []*links.ChainLink{
			orderApi,
		},
	}
	updateOrderFunction := &links.ChainLink{
		ResourceName: "updateOrderFunction",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/lambda/function"},
		},
		LinkImplementations: map[string]provider.Link{
			"ordersTable": lambdaDynamoDBTableLink,
		},
		Paths: []string{"/orderApi"},
		LinkedFrom: []*links.ChainLink{
			orderApi,
		},
	}

	dynamoDBTableStreamLink, _ := testProviderImpl.Link(context.TODO(), "aws/dynamodb/table", "aws/dynamodb/stream")
	// The only hard link in this chain is between the orders table
	// and the orders stream.
	ordersTable := &links.ChainLink{
		ResourceName: "ordersTable",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/dynamodb/table"},
		},
		LinkImplementations: map[string]provider.Link{
			"ordersStream": dynamoDBTableStreamLink,
		},
		Paths: []string{
			"/orderApi/getOrdersFunction",
			"/orderApi/createOrderFunction",
			"/orderApi/updateOrderFunction",
		},
		LinkedFrom: []*links.ChainLink{
			getOrdersFunction,
		},
	}

	dynamoDBStreamLambdaLink, _ := testProviderImpl.Link(context.TODO(), "aws/dynamodb/stream", "aws/lambda/function")
	ordersStream := &links.ChainLink{
		ResourceName: "ordersStream",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/dynamodb/stream"},
		},
		Paths: []string{
			"/orderApi/getOrdersFunction/ordersTable",
			"/orderApi/createOrderFunction/ordersTable",
			"/orderApi/updateOrderFunction/ordersTable",
		},
		LinkImplementations: map[string]provider.Link{
			"statsAccumulatorFunction": dynamoDBStreamLambdaLink,
		},
		LinkedFrom: []*links.ChainLink{
			getOrdersFunction,
			createOrderFunction,
			updateOrderFunction,
		},
		LinksTo: []*links.ChainLink{},
	}

	// Includes transitive soft circular link.
	statsAccumulatorFunction := &links.ChainLink{
		ResourceName: "statsAccumulatorFunction",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/lambda/function"},
		},
		Paths: []string{
			"/orderApi/getOrdersFunction/ordersTable/ordersStream",
			"/orderApi/createOrderFunction/ordersTable/ordersStream",
			"/orderApi/updateOrderFunction/ordersTable/ordersStream",
		},
		LinkImplementations: map[string]provider.Link{
			"ordersTable": lambdaDynamoDBTableLink,
		},
		LinkedFrom: []*links.ChainLink{
			ordersStream,
		},
	}

	resourceWithMissingLinkImplementation := &links.ChainLink{
		ResourceName: "secondaryOrdersDB",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/rds/dbInstance"},
		},
		Paths: []string{
			"/orderApi/getOrdersFunction/ordersTable/ordersStream/statsAccumulator",
			"/orderApi/createOrderFunction/ordersTable/ordersStream/statsAccumulator",
			"/orderApi/updateOrderFunction/ordersTable/ordersStream/statsAccumulator",
		},
		LinkImplementations: map[string]provider.Link{},
		LinkedFrom: []*links.ChainLink{
			statsAccumulatorFunction,
		},
	}

	orderApi.LinksTo = []*links.ChainLink{
		getOrdersFunction,
		createOrderFunction,
		updateOrderFunction,
	}
	getOrdersFunction.LinksTo = []*links.ChainLink{
		ordersTable,
	}
	createOrderFunction.LinksTo = []*links.ChainLink{
		ordersTable,
	}
	updateOrderFunction.LinksTo = []*links.ChainLink{
		ordersTable,
	}
	ordersTable.LinksTo = []*links.ChainLink{
		ordersStream,
	}
	ordersStream.LinksTo = []*links.ChainLink{
		statsAccumulatorFunction,
	}
	statsAccumulatorFunction.LinksTo = []*links.ChainLink{
		ordersTable,
		resourceWithMissingLinkImplementation,
	}

	return []*links.ChainLink{
		orderApi,
	}
}

var orderFixture2 = orderChainLinkFixture{
	inputChains: orderFixture2Chain(),
	expectedPresent: []string{
		"route1",
		"subnet1",
		"sg1",
		"routeTable1",
		"vpc1",
		"igw1",
	},
	orderedExpected: [][]string{
		{"routeTable1", "route1"},
		{"igw1", "route1"},
		{"vpc1", "routeTable1"},
		{"vpc1", "subnet1"},
		{"vpc1", "sg1"},
	},
}

func orderFixture2Chain() []*links.ChainLink {
	routeRouteTableLink, _ := testProviderImpl.Link(context.TODO(), "aws/ec2/route", "aws/ec2/routeTable")
	routeIGWLink, _ := testProviderImpl.Link(context.TODO(), "aws/ec2/route", "aws/ec2/internetGateway")
	route := &links.ChainLink{
		ResourceName: "route1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/route"},
		},
		Paths: []string{},
		LinkImplementations: map[string]provider.Link{
			"routeTable1":      routeRouteTableLink,
			"internetGateway1": routeIGWLink,
		},
		LinkedFrom: []*links.ChainLink{},
		LinksTo:    []*links.ChainLink{},
	}

	routeTableVPCLink, _ := testProviderImpl.Link(context.TODO(), "aws/ec2/routeTable", "aws/ec2/vpc")
	routeTable := &links.ChainLink{
		ResourceName: "routeTable1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/routeTable"},
		},
		Paths: []string{
			"/route1",
		},
		LinkImplementations: map[string]provider.Link{
			"vpc1": routeTableVPCLink,
		},
		LinkedFrom: []*links.ChainLink{
			route,
		},
		LinksTo: []*links.ChainLink{},
	}

	internetGateway := &links.ChainLink{
		ResourceName: "igw1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/internetGateway"},
		},
		Paths: []string{
			"/route1",
		},
		LinkImplementations: map[string]provider.Link{},
		LinkedFrom: []*links.ChainLink{
			route,
		},
		LinksTo: []*links.ChainLink{},
	}

	subnetVPCLink, _ := testProviderImpl.Link(context.TODO(), "aws/ec2/subnet", "aws/ec2/vpc")
	subnet := &links.ChainLink{
		ResourceName: "subnet1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/subnet"},
		},
		Paths: []string{},
		LinkImplementations: map[string]provider.Link{
			"vpc1": subnetVPCLink,
		},
		LinkedFrom: []*links.ChainLink{},
		LinksTo:    []*links.ChainLink{},
	}

	securityGroupLink, _ := testProviderImpl.Link(context.TODO(), "aws/ec2/securityGroup", "aws/ec2/vpc")
	securityGroup := &links.ChainLink{
		ResourceName: "sg1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/securityGroup"},
		},
		Paths: []string{},
		LinkImplementations: map[string]provider.Link{
			"vpc1": securityGroupLink,
		},
		LinkedFrom: []*links.ChainLink{},
		LinksTo:    []*links.ChainLink{},
	}

	vpc := &links.ChainLink{
		ResourceName: "vpc1",
		Resource: &schema.Resource{
			Type: &schema.ResourceTypeWrapper{Value: "aws/ec2/vpc"},
		},
		Paths: []string{
			"/route1/routeTable1",
			"/subnet1",
			"/sg1",
		},
		LinkImplementations: map[string]provider.Link{},
		LinkedFrom: []*links.ChainLink{
			routeTable,
			subnet,
			securityGroup,
		},
		LinksTo: []*links.ChainLink{},
	}

	route.LinksTo = []*links.ChainLink{
		routeTable,
		internetGateway,
	}
	routeTable.LinksTo = []*links.ChainLink{
		vpc,
	}
	subnet.LinksTo = []*links.ChainLink{
		vpc,
	}
	securityGroup.LinksTo = []*links.ChainLink{
		vpc,
	}

	return []*links.ChainLink{
		route,
		subnet,
		securityGroup,
	}
}

func TestOrderingTestSuite(t *testing.T) {
	suite.Run(t, new(OrderingTestSuite))
}
