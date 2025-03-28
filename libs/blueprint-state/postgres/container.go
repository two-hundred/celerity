package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type pgStateContainerImpl struct {
	instancesContainer *instancesContainerImpl
	resourcesContainer *resourcesContainerImpl
	linksContainer     *linksContainerImpl
	childrenContainer  *childrenContainerImpl
	metadataContainer  *metadataContainerImpl
	exportContainer    *exportContainerImpl
}

// LoadStateContainer loads a new state container
// that uses postgres for persistence.
//
// The postgres connection pool must be configured appropriately
// in the calling application where the application will take care of making
// sure the connection pool is closed when a command is finished or the application
// is shutting down.
func LoadStateContainer(
	ctx context.Context,
	connPool *pgxpool.Pool,
	logger core.Logger,
) (state.Container, error) {

	instancesContainer := &instancesContainerImpl{
		connPool: connPool,
	}

	container := &pgStateContainerImpl{
		instancesContainer: instancesContainer,
		resourcesContainer: &resourcesContainerImpl{
			connPool: connPool,
		},
		linksContainer: &linksContainerImpl{
			connPool: connPool,
		},
		childrenContainer: &childrenContainerImpl{
			connPool:  connPool,
			instances: instancesContainer,
		},
		metadataContainer: &metadataContainerImpl{
			connPool: connPool,
		},
		exportContainer: &exportContainerImpl{
			connPool: connPool,
		},
	}

	return container, nil
}

func (c *pgStateContainerImpl) Instances() state.InstancesContainer {
	return c.instancesContainer
}

func (c *pgStateContainerImpl) Resources() state.ResourcesContainer {
	return c.resourcesContainer
}

func (c *pgStateContainerImpl) Links() state.LinksContainer {
	return c.linksContainer
}

func (c *pgStateContainerImpl) Children() state.ChildrenContainer {
	return c.childrenContainer
}

func (c *pgStateContainerImpl) Metadata() state.MetadataContainer {
	return c.metadataContainer
}

func (c *pgStateContainerImpl) Exports() state.ExportsContainer {
	return c.exportContainer
}
