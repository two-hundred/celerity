package languageservices

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/two-hundred/celerity/libs/blueprint/container"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/schema"
	"github.com/two-hundred/celerity/tools/blueprint-ls/internal/blueprint"
	"github.com/two-hundred/celerity/tools/blueprint-ls/internal/diagnostichelpers"
	"github.com/two-hundred/ls-builder/common"
	lsp "github.com/two-hundred/ls-builder/lsp_3_17"
	"go.uber.org/zap"
)

// DiagnosticsService is a service that provides functionality
// for diagnostics.
type DiagnosticsService struct {
	state                  *State
	settingsService        *SettingsService
	diagnosticErrorService *DiagnosticErrorService
	loader                 container.Loader
	logger                 *zap.Logger
}

// NewDiagnosticsService creates a new service for diagnostics.
func NewDiagnosticsService(
	state *State,
	settingsService *SettingsService,
	diagnosticErrorService *DiagnosticErrorService,
	loader container.Loader,
	logger *zap.Logger,
) *DiagnosticsService {
	return &DiagnosticsService{
		state,
		settingsService,
		diagnosticErrorService,
		loader,
		logger,
	}
}

// ValidateTextDocument validates a text document and returns diagnostics.
func (s *DiagnosticsService) ValidateTextDocument(
	context *common.LSPContext,
	docURI lsp.URI,
) ([]lsp.Diagnostic, *schema.Blueprint, error) {
	diagnostics := []lsp.Diagnostic{}
	settings, err := s.settingsService.GetDocumentSettings(context, docURI)
	if err != nil {
		return nil, nil, err
	}
	s.logger.Debug(fmt.Sprintf("Settings: %v", settings))
	content := s.state.GetDocumentContent(docURI)
	if content == nil {
		return diagnostics, nil, nil
	}

	validationResult, err := s.loader.ValidateString(
		context.Context,
		*content,
		schema.YAMLSpecFormat,
		blueprint.NewParams(
			map[string]map[string]*core.ScalarValue{},
			map[string]*core.ScalarValue{},
			map[string]*core.ScalarValue{},
		),
	)
	s.logger.Info("Blueprint diagnostics: ")
	spew.Fdump(os.Stderr, validationResult.Diagnostics)
	diagnostics = append(
		diagnostics,
		diagnostichelpers.BlueprintToLSP(
			validationResult.Diagnostics,
		)...,
	)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Error loading blueprint: %v", err))
		errDiagnostics := s.diagnosticErrorService.BlueprintErrorToDiagnostics(
			err,
			docURI,
		)
		diagnostics = append(diagnostics, errDiagnostics...)
	}

	return diagnostics, validationResult.Schema, nil
}