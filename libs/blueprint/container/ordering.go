package container

import (
	"context"
	"fmt"
	"sort"
	"strings"

	bpcore "github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
	"github.com/two-hundred/celerity/libs/common/core"
)

// OrderLinksForDeployment deals with creating a flat ordered
// slice of chain links for stage changing and deployments.
// Ordering is determined by the priority resource type specified
// in each link implementation and usage of references between resources.
// A resource reference is treated as a hard link where the priority resource
// is the resource being referenced.
//
// It is a requirement for the input chains not to have any direct
// or transitive circular hard links.
// A hard link is when one resource type requires the other in a link
// relationship to be deployed first.
//
// For the following set of chains:
//
// (lt) = the linked to resource is the priority resource type.
// (lf) = the linked from resource is the priority resource type.
//
// *All the links in the example below are hard links.
//
// Chain 1
// ├── ResourceA1
// │	 ├── ResourceA2 (lf)
// │	 │   ├── ResourceA4 (lt)
// │	 │   └── ResourceA5 (lt)
// │	 └── ResourceA3 (lf)
// │	 	   └── ResourceA6 (lf)
//
// Chain 2
// ├── ResourceB1
// │	 ├── ResourceB2 (lt)
// │	 │   ├── ResourceB4 (lt)
// │     │   │   └── ResourceA6 (lt)
// │	 │   └── ResourceB5 (lt)
// │	 └── ResourceB3 (lf)
// │	 	   └── ResourceB6 (lt)
//
// We will want output like:
// [
//
//	ResourceA4,
//	ResourceA5,
//	ResourceA1,
//	ResourceA2,
//	ResourceA3,
//	ResourceA6,
//	ResourceB4,
//	ResourceB5,
//	ResourceB2,
//	ResourceB1,
//	ResourceB6,
//	ResourceB3
//
// ]
//
// What matters in the output is that resources are ordered by the priority
// definition of the links, the order of items that have no direct or transitive
// relationship are irrelevant.
func OrderLinksForDeployment(
	ctx context.Context,
	chains []*links.ChainLinkNode,
	refChainCollector validation.RefChainCollector,
	params bpcore.BlueprintParams,
) ([]*links.ChainLinkNode, error) {
	flattened := flattenChains(chains, []*links.ChainLinkNode{})
	var sortErr error
	sort.Slice(flattened, func(i, j int) bool {
		linkA := flattened[i]
		linkB := flattened[j]

		pathsWithLinkA := core.Filter(linkB.Paths, isResourceAncestor(linkA.ResourceName))
		linkAIsAncestor := len(pathsWithLinkA) > 0

		pathsWithLinkB := core.Filter(linkA.Paths, isResourceAncestor(linkB.ResourceName))
		linkAIsDescendant := len(pathsWithLinkB) > 0

		directParentsOfLinkB := getDirectParentsForPaths(pathsWithLinkA, linkB)

		// link A has priority in two cases.
		// 1, if at least one of the direct parents of link B
		// (for which link A is an ancestor) is the priority resource type
		// in the link relationship.
		// 2, if at least one of the direct children of link B
		// (for which link A is a descendant) is the priority resource type
		// in the link relationship.
		isParentWithPriority := len(core.Filter(directParentsOfLinkB, hasPriorityOver(ctx, linkB, params, &sortErr))) > 0
		if sortErr != nil {
			return false
		}
		isChildWithPriority := len(core.Filter(linkB.LinksTo, hasPriorityOver(ctx, linkB, params, &sortErr))) > 0
		if sortErr != nil {
			return false
		}
		// If A references B or any of B's descendants then A does not have priority regardless
		// of the link relationship. (An explicit reference is a dependency)
		linkAReferencesLinkB := linkResourceReferences(refChainCollector, linkA, linkB)
		linkAHasPriority := (isParentWithPriority || isChildWithPriority) && !linkAReferencesLinkB

		// If link B references link A but is not connected via a link relationship,
		// then link A has priority.
		// For example, let's say link A is an "orders" NoSQL table in a blueprint
		// and link B is a "createOrders" serverless function.
		// The "createOrders" function references the "orders" table in its environment variables
		// as the source for the table name made available to the function code.
		// There is no linkSelector initated link between the two resources, however, the "orders"
		// table (link A) needs to be deployed before the "createOrders" function (link B) so the function can source
		// the table name from the environment variables.
		linkBReferencesLinkA := linkResourceReferences(refChainCollector, linkB, linkA)

		return linkBReferencesLinkA || ((linkAIsAncestor || linkAIsDescendant) && linkAHasPriority)
	})
	return flattened, sortErr
}

func getDirectParentsForPaths(paths []string, link *links.ChainLinkNode) []*links.ChainLinkNode {
	return core.Filter(link.LinkedFrom, isLastInAtLeastOnePath(paths))
}

func isLastInAtLeastOnePath(paths []string) func(*links.ChainLinkNode, int) bool {
	return func(candidateParentLink *links.ChainLinkNode, index int) bool {
		return len(core.Filter(paths, isLastInPath(candidateParentLink))) > 0
	}
}

func isLastInPath(link *links.ChainLinkNode) func(string, int) bool {
	return func(path string, index int) bool {
		return strings.HasSuffix(path, fmt.Sprintf("/%s", link.ResourceName))
	}
}

func hasPriorityOver(
	ctx context.Context,
	otherLink *links.ChainLinkNode,
	params bpcore.BlueprintParams,
	captureError *error,
) func(*links.ChainLinkNode, int) bool {
	return func(candidatePriorityLink *links.ChainLinkNode, index int) bool {
		linkImplementation, hasLinkImplementation := candidatePriorityLink.LinkImplementations[otherLink.ResourceName]
		if !hasLinkImplementation {
			// The relationship could be either way.
			linkImplementation, hasLinkImplementation = otherLink.LinkImplementations[candidatePriorityLink.ResourceName]
		}

		if !hasLinkImplementation {
			// Might be a good idea to refactor this so we can return an error
			// somehow as something will be wrong somewhere in the code
			// if there is no link implementation.
			return false
		}

		priorityResourceTypeOutput, err := linkImplementation.GetPriorityResourceType(
			ctx,
			&provider.LinkGetPriorityResourceTypeInput{
				Params: params,
			},
		)
		if err != nil {
			*captureError = err
			return false
		}

		kindOutput, err := linkImplementation.GetKind(ctx, &provider.LinkGetKindInput{
			Params: params,
		})
		if err != nil {
			*captureError = err
			return false
		}
		isHardLink := kindOutput.Kind == provider.LinkKindHard
		return priorityResourceTypeOutput.PriorityResourceType == candidatePriorityLink.Resource.Type.Value && isHardLink
	}
}

func flattenChains(chains []*links.ChainLinkNode, flattenedAccum []*links.ChainLinkNode) []*links.ChainLinkNode {
	flattened := append([]*links.ChainLinkNode{}, flattenedAccum...)
	for _, chain := range chains {
		if !core.SliceContains(flattened, chain) {
			flattened = append(flattened, chain)
			if len(chain.LinksTo) > 0 {
				flattened = flattenChains(chain.LinksTo, flattened)
			}
		}
	}
	return flattened
}
