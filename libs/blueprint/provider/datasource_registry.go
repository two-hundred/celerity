package provider

import (
	"context"
	"sync"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/errors"
)

// DataSourceRegistry provides a way to retrieve data source plugins
// across multiple providers for tasks such as data source exports
// validation.
type DataSourceRegistry interface {
	// GetSpecDefinition returns the definition of a data source spec
	// in the registry that includes allowed parameters and return types.
	GetSpecDefinition(
		ctx context.Context,
		dataSourceType string,
		input *DataSourceGetSpecDefinitionInput,
	) (*DataSourceGetSpecDefinitionOutput, error)

	// GetFilterFields returns the fields that can be used in a filter for a data source.
	GetFilterFields(
		ctx context.Context,
		dataSourceType string,
		input *DataSourceGetFilterFieldsInput,
	) (*DataSourceGetFilterFieldsOutput, error)

	// GetTypeDescription returns the description of a data source type
	// in the registry.
	GetTypeDescription(
		ctx context.Context,
		dataSourceType string,
		input *DataSourceGetTypeDescriptionInput,
	) (*DataSourceGetTypeDescriptionOutput, error)

	// HasDataSourceType checks if a data source type is available in the registry.
	HasDataSourceType(ctx context.Context, dataSourceType string) (bool, error)

	// ListDataSourceTypes retrieves a list of all the data source types avaiable
	// in the registry.
	ListDataSourceTypes(ctx context.Context) ([]string, error)

	// CustomValidate allows for custom validation of a data source of a given type.
	CustomValidate(
		ctx context.Context,
		dataSourceType string,
		input *DataSourceValidateInput,
	) (*DataSourceValidateOutput, error)

	// Fetch retrieves the data from a data source using the provider
	// of the given type.
	Fetch(
		ctx context.Context,
		dataSourceType string,
		input *DataSourceFetchInput,
	) (*DataSourceFetchOutput, error)
}

type dataSourceRegistryFromProviders struct {
	providers       map[string]Provider
	dataSourceCache *core.Cache[DataSource]
	dataSourceTypes []string
	mu              sync.Mutex
}

// NewDataSourceRegistry creates a new DataSourceRegistry from a map of providers,
// matching against providers based on the data source type prefix.
func NewDataSourceRegistry(providers map[string]Provider) DataSourceRegistry {
	return &dataSourceRegistryFromProviders{
		providers:       providers,
		dataSourceCache: core.NewCache[DataSource](),
		dataSourceTypes: []string{},
	}
}

func (r *dataSourceRegistryFromProviders) GetSpecDefinition(
	ctx context.Context,
	dataSourceType string,
	input *DataSourceGetSpecDefinitionInput,
) (*DataSourceGetSpecDefinitionOutput, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		return nil, err
	}

	return dataSourceImpl.GetSpecDefinition(ctx, input)
}

func (r *dataSourceRegistryFromProviders) GetTypeDescription(
	ctx context.Context,
	dataSourceType string,
	input *DataSourceGetTypeDescriptionInput,
) (*DataSourceGetTypeDescriptionOutput, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		return nil, err
	}

	return dataSourceImpl.GetTypeDescription(ctx, input)
}

func (r *dataSourceRegistryFromProviders) GetFilterFields(
	ctx context.Context,
	dataSourceType string,
	input *DataSourceGetFilterFieldsInput,
) (*DataSourceGetFilterFieldsOutput, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		return nil, err
	}

	return dataSourceImpl.GetFilterFields(ctx, input)
}

func (r *dataSourceRegistryFromProviders) HasDataSourceType(ctx context.Context, dataSourceType string) (bool, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		if runErr, isRunErr := err.(*errors.RunError); isRunErr {
			if runErr.ReasonCode == ErrorReasonCodeProviderDataSourceTypeNotFound {
				return false, nil
			}
		}
		return false, err
	}
	return dataSourceImpl != nil, nil
}

func (r *dataSourceRegistryFromProviders) ListDataSourceTypes(ctx context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.dataSourceTypes) > 0 {
		return r.dataSourceTypes, nil
	}

	dataSourceTypes := []string{}
	for _, provider := range r.providers {
		types, err := provider.ListDataSourceTypes(ctx)
		if err != nil {
			return nil, err
		}

		dataSourceTypes = append(dataSourceTypes, types...)
	}

	r.dataSourceTypes = dataSourceTypes

	return dataSourceTypes, nil
}

func (r *dataSourceRegistryFromProviders) CustomValidate(
	ctx context.Context,
	dataSourceType string,
	input *DataSourceValidateInput,
) (*DataSourceValidateOutput, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		return nil, err
	}

	return dataSourceImpl.CustomValidate(ctx, input)
}

func (r *dataSourceRegistryFromProviders) Fetch(
	ctx context.Context,
	dataSourceType string,
	input *DataSourceFetchInput,
) (*DataSourceFetchOutput, error) {
	dataSourceImpl, err := r.getDataSourceType(ctx, dataSourceType)
	if err != nil {
		return nil, err
	}

	return dataSourceImpl.Fetch(ctx, input)
}

func (r *dataSourceRegistryFromProviders) getDataSourceType(ctx context.Context, dataSourceType string) (DataSource, error) {
	dataSource, cached := r.dataSourceCache.Get(dataSourceType)
	if cached {
		return dataSource, nil
	}

	providerNamespace := ExtractProviderFromItemType(dataSourceType)
	provider, ok := r.providers[providerNamespace]
	if !ok {
		return nil, errDataSourceTypeProviderNotFound(providerNamespace, dataSourceType)
	}
	dataSourceImpl, err := provider.DataSource(ctx, dataSourceType)
	if err != nil {
		return nil, errProviderDataSourceTypeNotFound(dataSourceType, providerNamespace)
	}
	r.dataSourceCache.Set(dataSourceType, dataSourceImpl)

	return dataSourceImpl, nil
}
