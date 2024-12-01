package container

import (
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

func (s *ResourceChangeStagerTestSuite) resourceInfoFixture1() *provider.ResourceInfo {

	return &provider.ResourceInfo{
		ResourceID:               "test-resource-1",
		InstanceID:               "test-instance-1",
		ResourceName:             "complexResource",
		CurrentResourceState:     s.resourceInfoFixture1CurrentState(),
		ResourceWithResolvedSubs: s.resourceInfoFixture1NewResolvedResource(),
	}
}

func (s *ResourceChangeStagerTestSuite) resourceInfoFixture1CurrentState() *state.ResourceState {
	itemID := "test-item-1"
	currentEndpoint1 := "http://example.com/1"
	currentEndpoint2 := "http://example.com/2"
	currentEndpoint3 := "http://example.com/3"
	currentEndpoint4 := "http://example.com/4"
	currentEndpoint5 := "http://example.com/5"
	currentPrimaryPort := 8080
	currentIpv4Enabled := true
	currentSpecMetadataValue1 := "value1"
	currentSpecMetadataValue2 := "value2"
	currentMetadataCustomURL := "http://example.com"
	currentMetadataProtocol1 := "https"
	currentMetadataProtocol2 := "wss"
	otherItemValue := "other-item-value"
	vendorTag1 := "vendor-tag-1"
	vendorTag2 := "vendor-tag-2"
	vendorTag3 := "vendor-tag-3"
	localTag1 := "local-tag-1"
	localTag2 := "local-tag-2"

	return &state.ResourceState{
		ResourceID:                 "test-resource-1",
		ResourceName:               "complexResource",
		Status:                     core.ResourceStatusCreated,
		PreciseStatus:              core.PreciseResourceStatusCreated,
		LastDeployedTimestamp:      1732969676,
		LastDeployAttemptTimestamp: 1732969676,
		ResourceSpecData: &core.MappingNode{
			Fields: map[string]*core.MappingNode{
				"id": {
					Literal: &core.ScalarValue{
						StringValue: &itemID,
					},
				},
				"itemConfig": {
					Fields: map[string]*core.MappingNode{
						"endpoints": {
							Items: []*core.MappingNode{
								{
									Literal: &core.ScalarValue{
										StringValue: &currentEndpoint1,
									},
								},
								{
									Literal: &core.ScalarValue{
										StringValue: &currentEndpoint2,
									},
								},
								{
									Literal: &core.ScalarValue{
										StringValue: &currentEndpoint3,
									},
								},
								{
									Literal: &core.ScalarValue{
										StringValue: &currentEndpoint4,
									},
								},
								{
									Literal: &core.ScalarValue{
										StringValue: &currentEndpoint5,
									},
								},
							},
						},
						"primaryPort": {
							Literal: &core.ScalarValue{
								IntValue: &currentPrimaryPort,
							},
						},
						"ipv4": {
							Literal: &core.ScalarValue{
								BoolValue: &currentIpv4Enabled,
							},
						},
						"metadata": {
							Fields: map[string]*core.MappingNode{
								"value1": {
									Literal: &core.ScalarValue{
										StringValue: &currentSpecMetadataValue1,
									},
								},
								"value2": {
									Literal: &core.ScalarValue{
										StringValue: &currentSpecMetadataValue2,
									},
								},
							},
						},
					},
				},
				"otherItemConfig": {
					Literal: &core.ScalarValue{
						StringValue: &otherItemValue,
					},
				},
				"vendorTags": {
					Items: []*core.MappingNode{
						{
							Literal: &core.ScalarValue{
								StringValue: &vendorTag1,
							},
						},
						{
							Literal: &core.ScalarValue{
								StringValue: &vendorTag2,
							},
						},
						{
							Literal: &core.ScalarValue{
								StringValue: &vendorTag3,
							},
						},
					},
				},
			},
		},
		Metadata: &state.ResourceMetadataState{
			DisplayName: "Test Complex Resource",
			Annotations: map[string]string{
				"test.annotation.v1":          "first-annotation-value",
				"test.annotation.v2":          "second-annotation-value",
				"test.annotation.original-v3": "original-annotation-value",
			},
			Labels: map[string]string{
				"app":   "test-app-v1",
				"squad": "portal-squad",
			},
			Custom: &core.MappingNode{
				Fields: map[string]*core.MappingNode{
					"url": {
						Literal: &core.ScalarValue{
							StringValue: &currentMetadataCustomURL,
						},
					},
					"protocol": {
						Items: []*core.MappingNode{
							{
								Literal: &core.ScalarValue{
									StringValue: &currentMetadataProtocol1,
								},
							},
							{
								Literal: &core.ScalarValue{
									StringValue: &currentMetadataProtocol2,
								},
							},
						},
					},
					"localTags": {
						Items: []*core.MappingNode{
							{
								Literal: &core.ScalarValue{
									StringValue: &localTag1,
								},
							},
							{
								Literal: &core.ScalarValue{
									StringValue: &localTag2,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *ResourceChangeStagerTestSuite) resourceInfoFixture1NewResolvedResource() *provider.ResolvedResource {
	newDisplayName := "Test Complex Resource Updated"
	secondAnnotationValue := "second-annotation-value"
	thirdAnnotationValue := "third-annotation-value"
	newEndpoint1 := "http://example.com/new/1"
	newEndpoint2 := "http://example.com/new/2"
	newEndpoint4 := "http://example.com/4"
	newPrimaryPort := 8081
	newIpv4Enabled := false
	newSpecMetadataValue1 := "new-value1"
	newScore := 1.309
	newMetadataProtocol := "https"
	otherItemValue1 := "other-item-value-1"
	otherItemValue2 := "other-item-value-2"
	vendorTag := "vendor-tag-1"
	localTag := "local-tag-1"

	return &provider.ResolvedResource{
		Type: &schema.ResourceTypeWrapper{
			Value: "example/complex",
		},
		Metadata: &provider.ResolvedResourceMetadata{
			DisplayName: &core.MappingNode{
				Literal: &core.ScalarValue{
					StringValue: &newDisplayName,
				},
			},
			Annotations: &core.MappingNode{
				Fields: map[string]*core.MappingNode{
					// To be resolved on deployment
					"test.annotation.v1": (*core.MappingNode)(nil),
					"test.annotation.v2": {
						Literal: &core.ScalarValue{
							StringValue: &secondAnnotationValue,
						},
					},
					"test.annotation.v3": {
						Literal: &core.ScalarValue{
							StringValue: &thirdAnnotationValue,
						},
					},
				},
			},
			Labels: &schema.StringMap{
				Values: map[string]string{
					"app": "test-app-v2",
					"env": "production",
				},
			},
			Custom: &core.MappingNode{
				Fields: map[string]*core.MappingNode{
					// To be resolved on deployment
					"url": (*core.MappingNode)(nil),
					"protocol": {
						Literal: &core.ScalarValue{
							StringValue: &newMetadataProtocol,
						},
					},
					"localTags": {
						Items: []*core.MappingNode{
							{
								Literal: &core.ScalarValue{
									StringValue: &localTag,
								},
							},
						},
					},
				},
			},
		},
		Spec: &core.MappingNode{
			Fields: map[string]*core.MappingNode{
				"itemConfig": {
					Fields: map[string]*core.MappingNode{
						"endpoints": {
							Items: []*core.MappingNode{
								{
									Literal: &core.ScalarValue{
										StringValue: &newEndpoint1,
									},
								},
								{
									Literal: &core.ScalarValue{
										StringValue: &newEndpoint2,
									},
								},
								// To be resolved on deployment
								(*core.MappingNode)(nil),
								{
									Literal: &core.ScalarValue{
										StringValue: &newEndpoint4,
									},
								},
								// To be resolved on deployment
								(*core.MappingNode)(nil),
							},
						},
						"primaryPort": {
							Literal: &core.ScalarValue{
								IntValue: &newPrimaryPort,
							},
						},
						"ipv4": {
							Literal: &core.ScalarValue{
								BoolValue: &newIpv4Enabled,
							},
						},
						"score": {
							Literal: &core.ScalarValue{
								FloatValue: &newScore,
							},
						},
						"metadata": {
							Fields: map[string]*core.MappingNode{
								"value1": {
									Literal: &core.ScalarValue{
										StringValue: &newSpecMetadataValue1,
									},
								},
								// "value2" key/value pair has been removed.
							},
						},
					},
				},
				"otherItemConfig": {
					Fields: map[string]*core.MappingNode{
						"value1": {
							Literal: &core.ScalarValue{
								StringValue: &otherItemValue1,
							},
						},
						"value2": {
							Literal: &core.ScalarValue{
								StringValue: &otherItemValue2,
							},
						},
					},
				},
				"vendorTags": {
					Items: []*core.MappingNode{
						{
							Literal: &core.ScalarValue{
								StringValue: &vendorTag,
							},
						},
					},
				},
			},
		},
	}
}