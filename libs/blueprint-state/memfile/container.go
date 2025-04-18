package memfile

import (
	"sync"

	"github.com/spf13/afero"
	"github.com/two-hundred/celerity/libs/blueprint/core"
	"github.com/two-hundred/celerity/libs/blueprint/state"
)

type mfStateContainerImpl struct {
	instancesContainer *instancesContainerImpl
	resourcesContainer *resourcesContainerImpl
	linksContainer     *linksContainerImpl
	childrenContainer  *childrenContainerImpl
	metadataContainer  *metadataContainerImpl
	exportContainer    *exportContainerImpl
	persister          *statePersister
}

// Option is a type for options that can be passed to LoadStateContainer
// when creating an in-memory state container with file persistence.
type Option func(*mfStateContainerImpl)

// WithMaxGuideFileSize sets a guide for the maximum size of a state chunk file in bytes.
// If a single record (instance or resource drift entry) exceeds this size,
// it will not be split into multiple files.
// This is only a guide, the actual size of the files are often likely to be larger.
//
// When not set, the default value is 1MB (1,048,576 bytes).
func WithMaxGuideFileSize(maxGuideFileSize int64) func(*mfStateContainerImpl) {
	return func(p *mfStateContainerImpl) {
		p.persister.maxGuideFileSize = maxGuideFileSize
	}
}

// LoadStateContainer loads a new state container
// that uses in-process memory to store state
// with local files used for persistence.
//
// This will load the state into memory from the given directory
// as the initial state and will write state files to the same
// directory as they are updated.
// stateDir can be relative to the current working directory
// or an absolute path.
func LoadStateContainer(
	stateDir string,
	fs afero.Fs,
	logger core.Logger,
	opts ...Option,
) (state.Container, error) {
	mu := &sync.RWMutex{}

	state, err := loadStateFromDir(stateDir, fs)
	if err != nil {
		return nil, err
	}

	persister := &statePersister{
		stateDir:               stateDir,
		fs:                     fs,
		instanceIndex:          state.instanceIndex,
		resourceDriftIndex:     state.resourceDriftIndex,
		maxGuideFileSize:       DefaultMaxGuideFileSize,
		lastInstanceChunk:      getLastChunkFromIndex(state.instanceIndex),
		lastResourceDriftChunk: getLastChunkFromIndex(state.resourceDriftIndex),
	}

	container := &mfStateContainerImpl{
		persister: persister,
		instancesContainer: &instancesContainerImpl{
			instances: state.instances,
			// The instance ID lookup is not something that is persisted,
			// it is generated at load time as for the vast majority of use-cases
			// there will not be a significant cost to generating it on load.
			instanceIDLookup: createInstanceIDLookup(state.instances),
			resources:        state.resources,
			links:            state.links,
			fs:               fs,
			persister:        persister,
			logger:           logger,
			mu:               mu,
		},
		resourcesContainer: &resourcesContainerImpl{
			resources:            state.resources,
			resourceDriftEntries: state.resourceDrift,
			instances:            state.instances,
			fs:                   fs,
			persister:            persister,
			logger:               logger,
			mu:                   mu,
		},
		linksContainer: &linksContainerImpl{
			links:     state.links,
			instances: state.instances,
			fs:        fs,
			persister: persister,
			logger:    logger,
			mu:        mu,
		},
		childrenContainer: &childrenContainerImpl{
			instances: state.instances,
			fs:        fs,
			persister: persister,
			logger:    logger,
			mu:        mu,
		},
		metadataContainer: &metadataContainerImpl{
			instances: state.instances,
			fs:        fs,
			persister: persister,
			logger:    logger,
			mu:        mu,
		},
		exportContainer: &exportContainerImpl{
			instances: state.instances,
			fs:        fs,
			persister: persister,
			logger:    logger,
			mu:        mu,
		},
	}

	for _, opt := range opts {
		opt(container)
	}

	return container, nil
}

func (c *mfStateContainerImpl) Instances() state.InstancesContainer {
	return c.instancesContainer
}

func (c *mfStateContainerImpl) Resources() state.ResourcesContainer {
	return c.resourcesContainer
}

func (c *mfStateContainerImpl) Links() state.LinksContainer {
	return c.linksContainer
}

func (c *mfStateContainerImpl) Children() state.ChildrenContainer {
	return c.childrenContainer
}

func (c *mfStateContainerImpl) Metadata() state.MetadataContainer {
	return c.metadataContainer
}

func (c *mfStateContainerImpl) Exports() state.ExportsContainer {
	return c.exportContainer
}

func getLastChunkFromIndex(index map[string]*indexLocation) int {
	lastChunk := 0
	for _, locationInfo := range index {
		if locationInfo.ChunkNumber > lastChunk {
			lastChunk = locationInfo.ChunkNumber
		}
	}
	return lastChunk
}

func createInstanceIDLookup(
	instances map[string]*state.InstanceState,
) map[string]string {
	instanceIDLookup := make(map[string]string)
	for instanceID, instance := range instances {
		if instance.InstanceName != "" {
			instanceIDLookup[instance.InstanceName] = instanceID
		}
	}
	return instanceIDLookup
}
