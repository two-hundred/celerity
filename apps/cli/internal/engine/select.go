package engine

import (
	"github.com/two-hundred/celerity/apps/cli/internal/config"
	"github.com/two-hundred/celerity/libs/blueprint/container"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	"github.com/two-hundred/celerity/libs/blueprint/transform"
	"github.com/two-hundred/celerity/libs/blueprint/validation"
	"github.com/two-hundred/celerity/libs/build-engine/core"
	"go.uber.org/zap"
)

// Select returns the appropriate build engine based on the configuration
// provided.
func Select(confProvider *config.Provider, logger *zap.Logger) core.BuildEngine {
	if embedded, _ := confProvider.GetBool("embeddedEngine"); embedded {
		loader := container.NewDefaultLoader(
			map[string]provider.Provider{},
			map[string]transform.SpecTransformer{},
			/* stateContainer */ nil,
			/* updateChan */ nil,
			validation.NewRefChainCollector,
			container.WithLoaderTransformSpec(false),
			container.WithLoaderValidateAfterTransform(false),
			container.WithLoaderValidateRuntimeValues(false),
		)
		return core.NewDefaultBuildEngine(loader, logger)
	}
	connectProtocol, _ := confProvider.GetString("connectProtocol")
	return NewEngineAPI(connectProtocol)
}
