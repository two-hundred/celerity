package plugin

import (
	"errors"
	"fmt"

	"github.com/two-hundred/celerity/libs/build-engine/plugin/pluginservice"
	"github.com/two-hundred/celerity/libs/build-engine/plugin/providerserverv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// CreatePluginInstance is a function that creates a new instance of a plugin.
// This implements the pluginservice.PluginFactory interface.
func CreatePluginInstance(info *pluginservice.PluginInstanceInfo) (interface{}, func(), error) {
	if info.PluginType == pluginservice.PluginType_PLUGIN_TYPE_PROVIDER && info.ProtocolVersion == 1 {
		return createV1ProviderPlugin(info)
	}

	return nil, nil, errors.New("unsupported plugin type or protocol version")
}

func createV1ProviderPlugin(info *pluginservice.PluginInstanceInfo) (interface{}, func(), error) {

	conn, err := createGRPCConnection(info)
	closeConn := func() {
		conn.Close()
	}
	if err != nil {
		return nil, closeConn, err
	}

	client := providerserverv1.NewProviderClient(conn)
	// Give the build engine an instance of the provider.Provider
	// interface for the blueprint framework to interact with,
	// this plays into a more seamless integration with the build engine
	// and the blueprint framework, allowing for an instance of the build engine
	// to opt out of using the gRPC server plugin system.
	wrapped := providerserverv1.WrapProviderClient(client)
	return wrapped, closeConn, nil
}

func createGRPCConnection(info *pluginservice.PluginInstanceInfo) (*grpc.ClientConn, error) {
	if info.UnixSocketPath != "" {
		return grpc.NewClient("unix://"+info.UnixSocketPath, grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		))
	}
	return grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", info.TCPPort), grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
}