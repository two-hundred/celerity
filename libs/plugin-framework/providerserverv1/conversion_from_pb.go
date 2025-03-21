package providerserverv1

import (
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/serialisation"
	"github.com/two-hundred/celerity/libs/blueprint/state"
	"github.com/two-hundred/celerity/libs/plugin-framework/convertv1"
	sharedtypesv1 "github.com/two-hundred/celerity/libs/plugin-framework/sharedtypesv1"
)

func fromPBLinkIntermediaryResourcesCompleteResponse(
	response *UpdateLinkIntermediaryResourcesCompleteResponse,
) (*provider.LinkUpdateIntermediaryResourcesOutput, error) {
	intermediaryResourceStates, err := fromPBLinkIntermediaryResourceStates(
		response.IntermediaryResourceStates,
	)
	if err != nil {
		return nil, err
	}

	linkData, err := serialisation.FromMappingNodePB(
		response.LinkData,
		/* optional */ true,
	)
	if err != nil {
		return nil, err
	}

	return &provider.LinkUpdateIntermediaryResourcesOutput{
		IntermediaryResourceStates: intermediaryResourceStates,
		LinkData:                   linkData,
	}, nil
}

func fromPBLinkIntermediaryResourceStates(
	intermediaryResourceStates []*LinkIntermediaryResourceState,
) ([]*state.LinkIntermediaryResourceState, error) {
	var states []*state.LinkIntermediaryResourceState
	for _, state := range intermediaryResourceStates {
		intermediaryResourceState, err := fromPBLinkIntermediaryResourceState(state)
		if err != nil {
			return nil, err
		}
		states = append(states, intermediaryResourceState)
	}
	return states, nil
}

func fromPBLinkIntermediaryResourceState(
	pbState *LinkIntermediaryResourceState,
) (*state.LinkIntermediaryResourceState, error) {
	resourceSpecData, err := serialisation.FromMappingNodePB(
		pbState.ResourceSpecData,
		/* optional */ true,
	)
	if err != nil {
		return nil, err
	}

	return &state.LinkIntermediaryResourceState{
		ResourceID: pbState.ResourceId,
		InstanceID: pbState.InstanceId,
		Status:     core.ResourceStatus(pbState.Status),
		PreciseStatus: core.PreciseResourceStatus(
			pbState.PreciseStatus,
		),
		LastDeployedTimestamp:      int(pbState.LastDeployedTimestamp),
		LastDeployAttemptTimestamp: int(pbState.LastDeployAttemptTimestamp),
		ResourceSpecData:           resourceSpecData,
		FailureReasons:             pbState.FailureReasons,
	}, nil
}

func fromPBLinkPriorityResourceInfo(
	pbPriorityInfo *LinkPriorityResourceInfo,
) *provider.LinkGetPriorityResourceOutput {
	return &provider.LinkGetPriorityResourceOutput{
		PriorityResource: provider.LinkPriorityResource(
			pbPriorityInfo.PriorityResource,
		),
		PriorityResourceType: convertv1.ResourceTypeToString(
			pbPriorityInfo.PriorityResourceType,
		),
	}
}

func fromPBTypeDescriptionForLink(
	req *sharedtypesv1.TypeDescription,
) *provider.LinkGetTypeDescriptionOutput {
	if req == nil {
		return nil
	}

	return &provider.LinkGetTypeDescriptionOutput{
		PlainTextDescription: req.PlainTextDescription,
		MarkdownDescription:  req.MarkdownDescription,
		PlainTextSummary:     req.PlainTextSummary,
		MarkdownSummary:      req.MarkdownSummary,
	}
}
