package validation

import (
	"context"
	"strings"

	bpcore "github.com/two-hundred/celerity/libs/blueprint/pkg/core"
	"github.com/two-hundred/celerity/libs/blueprint/pkg/schema"
	"github.com/two-hundred/celerity/libs/blueprint/pkg/source"
	"github.com/two-hundred/celerity/libs/common/pkg/core"
)

// ValidateTransforms checks for non-standard transforms and reports warnings
// when the spec is not going to be transformed (e.g. dry run validation).
func ValidateTransforms(
	ctx context.Context,
	blueprint *schema.Blueprint,
	specWillBeTransformed bool,
) ([]*bpcore.Diagnostic, error) {
	diagnostics := []*bpcore.Diagnostic{}
	if specWillBeTransformed || blueprint.Transform == nil {
		// Errors for missing or invalid transforms will
		// be caught on collection of transform implementations.
		return diagnostics, nil
	}

	for i, transform := range blueprint.Transform.Values {
		if strings.TrimSpace(transform) == "" {
			diagnostics = append(diagnostics, &bpcore.Diagnostic{
				Level:   bpcore.DiagnosticLevelError,
				Message: "A transform can not be empty.",
				Range:   diagnosticRangeFromTransform(i, blueprint),
			})
		} else if !core.SliceContainsComparable(CoreTransforms, transform) {
			diagnostics = append(diagnostics, &bpcore.Diagnostic{
				Level: bpcore.DiagnosticLevelWarning,
				Message: "The transform \"" + transform + "\" is not a core transform," +
					" you will need to make sure it is configured when deploying this blueprint.",
				Range: diagnosticRangeFromTransform(i, blueprint),
			})
		}
	}

	return diagnostics, nil
}

func diagnosticRangeFromTransform(transformIndex int, blueprint *schema.Blueprint) *bpcore.DiagnosticRange {
	if len(blueprint.Transform.SourceMeta) == 0 {
		return &bpcore.DiagnosticRange{
			Start: &source.Meta{
				Line:   1,
				Column: 1,
			},
			End: &source.Meta{
				Line:   1,
				Column: 1,
			},
		}
	}

	transformSourceMeta := blueprint.Transform.SourceMeta[transformIndex]
	endSourceMeta := &source.Meta{
		Line:   transformSourceMeta.Line + 1,
		Column: 1,
	}
	if transformIndex+1 < len(blueprint.Transform.SourceMeta) {
		endSourceMeta = &source.Meta{
			Line:   blueprint.Transform.SourceMeta[transformIndex+1].Line,
			Column: 1,
		}
	}

	return &bpcore.DiagnosticRange{
		Start: transformSourceMeta,
		End:   endSourceMeta,
	}
}

const (
	// TransformCelerity2024_09_01 is the transform to be used for
	// Celerity resources that provide an abstraction over a more complex
	// combination of underlying resources.
	TransformCelerity2024_09_01 = "celerity-2024-09-01"
)

var (
	// CoreTransforms is the list of transforms that are considered to be core
	// to Celerity, these will be transforms maintained by the Celerity team
	// or by trusted maintainers.
	CoreTransforms = []string{
		TransformCelerity2024_09_01,
	}
)
