package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/links"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
)

func deriveSpecFormat(specFilePath string) (schema.SpecFormat, error) {
	// Bear in mind this is a somewhat naive check, however if the spec file data
	// isn't valid YAML or JSON it will be caught in a failure to unmarshal
	// the spec.
	if strings.HasSuffix(specFilePath, ".yml") || strings.HasSuffix(specFilePath, ".yaml") {
		return schema.YAMLSpecFormat, nil
	}

	if strings.HasSuffix(specFilePath, ".json") {
		return schema.JSONSpecFormat, nil
	}

	return "", errUnsupportedSpecFileExtension(specFilePath)
}

// Provide a function compatible with loadSpec that simply returns an already defined format.
// This is useful for using the same functionality for loading from a string and from disk.
func predefinedFormatFactory(predefinedFormat schema.SpecFormat) func(input string) (schema.SpecFormat, error) {
	return func(input string) (schema.SpecFormat, error) {
		return predefinedFormat, nil
	}
}

func copyProviderMap(m map[string]provider.Provider) map[string]provider.Provider {
	copy := make(map[string]provider.Provider, len(m))
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

func collectLinksFromChain(
	ctx context.Context,
	chain *links.ChainLink,
	refChainCollector validation.RefChainCollector,
) error {
	referencedByResourceID := fmt.Sprintf("resources.%s", chain.ResourceName)
	for _, link := range chain.LinksTo {
		linkImplementation, err := getLinkImplementation(chain, link)
		if err != nil {
			return err
		}

		linkKindOutput, err := linkImplementation.GetKind(ctx, &provider.LinkGetKindInput{})
		if err != nil {
			return err
		}

		// Only collect link for cycle detection if it is a hard link.
		// Soft links do not require a specific order of deployment/resolution.
		if linkKindOutput.Kind == provider.LinkKindHard {
			resourceID := fmt.Sprintf("resources.%s", link.ResourceName)
			err = refChainCollector.Collect(resourceID, link, referencedByResourceID, []string{"link"})
			if err != nil {
				return err
			}
		}

		// There is no risk of infinite recursion due to cyclic links as at this point,
		// any pure link cycles have been detected and reported.
		err = collectLinksFromChain(ctx, link, refChainCollector)
		if err != nil {
			return err
		}
	}

	return nil
}

func getLinkImplementation(
	linkA *links.ChainLink,
	linkB *links.ChainLink,
) (provider.Link, error) {
	linkImplementation, hasLinkImplementation := linkA.LinkImplementations[linkB.ResourceName]
	if !hasLinkImplementation {
		// The relationship could be either way.
		linkImplementation, hasLinkImplementation = linkB.LinkImplementations[linkA.ResourceName]
	}

	if !hasLinkImplementation {
		return nil, fmt.Errorf("no link implementation found between %s and %s", linkA.ResourceName, linkB.ResourceName)
	}

	return linkImplementation, nil
}
