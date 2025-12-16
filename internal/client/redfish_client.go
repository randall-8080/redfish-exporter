package client

import (
	"github.com/e-breuninger/redfish-exporter/internal/config"
	"github.com/stmcginnis/gofish"
)

func NewClientFromModuleConfig(module config.Module, target string) (*gofish.APIClient, error) {
	config := gofish.ClientConfig{
		Endpoint:         target,
		Username:         module.Auth.Username,
		Password:         module.Auth.Password,
		Insecure:         module.Insecure,
		ReuseConnections: true,
	}

	client, err := gofish.Connect(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}
