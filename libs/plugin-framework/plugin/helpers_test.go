package plugin

import (
	"path/filepath"

	"github.com/spf13/afero"
	"github.com/two-hundred/celerity/libs/plugin-framework/internal/testutils"
	"github.com/two-hundred/celerity/libs/plugin-framework/pluginservicev1"
	"github.com/two-hundred/celerity/libs/plugin-framework/utils"
)

var (
	testPluginRootPaths = []string{
		"/root/.celerity/deploy-engine/plugins/bin",
		"/usr/local/celerity/deploy-engine/plugins/bin",
	}
)

func loadPluginsIntoFS(plugins []*PluginPathInfo, fs afero.Fs) error {
	for _, pluginPath := range plugins {
		fs.MkdirAll(filepath.Dir(pluginPath.AbsolutePath), 0755)
		err := afero.WriteFile(fs, pluginPath.AbsolutePath, []byte{1, 1, 1, 1}, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadExpectedPluginPaths() []*PluginPathInfo {
	return []*PluginPathInfo{
		{
			AbsolutePath: "/root/.celerity/deploy-engine/plugins/bin/providers/celerity/aws/1.0.0/plugin",
			PluginType:   "provider",
			ID:           "celerity/aws",
			Version:      "1.0.0",
		},
		{
			AbsolutePath: "/root/.celerity/deploy-engine/plugins/bin/transformers/celerity/celerity/2.0.1/plugin",
			PluginType:   "transformer",
			ID:           "celerity/celerity",
			Version:      "2.0.1",
		},
		{
			AbsolutePath: "/usr/local/celerity/deploy-engine/plugins/bin/providers" +
				"/registry.customhost.com/celerity/azure/3.2.0/plugin",
			PluginType: "provider",
			ID:         "registry.customhost.com/celerity/azure",
			Version:    "3.2.0",
		},
	}
}

type mockPluginManager struct {
	pluginMap         map[pluginservicev1.PluginType]map[string]*pluginservicev1.PluginInstance
	testTransformName string
}

func (m *mockPluginManager) GetPlugins(
	pluginType pluginservicev1.PluginType,
) []*pluginservicev1.PluginInstance {
	instances := []*pluginservicev1.PluginInstance{}
	for _, instance := range m.pluginMap[pluginType] {
		instances = append(instances, instance)
	}
	return instances
}

func (m *mockPluginManager) GetPlugin(
	pluginType pluginservicev1.PluginType,
	id string,
) *pluginservicev1.PluginInstance {
	return m.pluginMap[pluginType][id]
}

func (m *mockPluginManager) RegisterPlugin(
	pluginInstanceInfo *pluginservicev1.PluginInstanceInfo,
) error {
	client := createMockPluginClient(pluginInstanceInfo, m.testTransformName)
	instance := &pluginservicev1.PluginInstance{
		Info:   pluginInstanceInfo,
		Client: client,
		CloseConn: func() {
			// Do nothing as the plugin is a stub for launch testing.
		},
	}
	m.pluginMap[pluginInstanceInfo.PluginType][pluginInstanceInfo.ID] = instance
	return nil
}

func (m *mockPluginManager) DeregisterPlugin(
	pluginType pluginservicev1.PluginType,
	id string,
) error {
	delete(m.pluginMap[pluginType], id)
	return nil
}

type mockPluginExecutor struct {
	pluginManager pluginservicev1.Manager
	// A mapping of plugin paths to the number of times they should be
	// attempted before they register with the plugin manager.
	registerOnAttempt map[string]int
	// A mapping of plugin paths to the number of times they have been
	// attempted to register with the plugin manager.
	registerAttempts map[string]int
	// A mapping of plugin paths to the plugin instance info used
	// to register the plugin with the plugin manager.
	pluginInstances map[string]*pluginservicev1.PluginInstanceInfo
}

func (e *mockPluginExecutor) Execute(pluginPath string) (PluginProcess, error) {
	attempts, hasAttempts := e.registerAttempts[pluginPath]
	if !hasAttempts {
		e.registerAttempts[pluginPath] = 0
	}

	if attempts < e.registerOnAttempt[pluginPath] {
		e.registerAttempts[pluginPath] = attempts + 1
		return &mockPluginProcess{}, nil
	}

	pluginInstanceInfo := e.pluginInstances[pluginPath]
	err := e.pluginManager.RegisterPlugin(pluginInstanceInfo)
	if err != nil {
		return nil, err
	}

	return &mockPluginProcess{}, nil
}

type mockPluginProcess struct{}

func (p *mockPluginProcess) Kill() error {
	return nil
}

func createMockPluginClient(
	pluginInfo *pluginservicev1.PluginInstanceInfo,
	transformName string,
) any {
	if pluginInfo.PluginType == pluginservicev1.PluginType_PLUGIN_TYPE_PROVIDER {
		return &testutils.MockProvider{
			ProviderNamespace: utils.ExtractProviderNamespace(pluginInfo.ID),
		}
	}

	return &testutils.MockTransformer{
		TransformName: transformName,
	}
}
