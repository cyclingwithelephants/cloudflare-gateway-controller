package cf

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
)

const (
	ConfigSourceLocal = "local"
)

type Api struct {
	ApiToken                    string
	Client                      *cloudflare.API
	Ctx                         context.Context
	CloudflareResourceContainer *cloudflare.ResourceContainer
}

func NewAPI(
	ctx context.Context,
	apiToken string,
	cloudflareAccountId string,
) (*Api, error) {
	client, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize cloudflare client")
	}

	return &Api{
		Client:                      client,
		Ctx:                         ctx,
		CloudflareResourceContainer: cloudflare.AccountIdentifier(cloudflareAccountId),
	}, nil
}

func (api *Api) CreateTunnel(name string, secret string) (cloudflare.Tunnel, error) {
	tunnel, err := api.Client.CreateTunnel(
		api.Ctx,
		api.CloudflareResourceContainer,
		newTunnelCreateParams(name, secret),
	)
	if err != nil {
		return cloudflare.Tunnel{}, errors.Wrap(err, "failed to create tunnel")
	}
	return tunnel, nil
}

func (api *Api) DeleteTunnel(tunnelID string) error {
	// Deletes any inactive connections on a tunnel
	err := api.Client.CleanupTunnelConnections(api.Ctx, api.CloudflareResourceContainer, tunnelID)
	if err != nil {
		return errors.Wrap(err, "failed to cleanup tunnel connections")
	}

	err = api.Client.DeleteTunnel(api.Ctx, api.CloudflareResourceContainer, tunnelID)
	if err != nil {
		return errors.Wrap(err, "failed to delete tunnel")
	}

	return nil
}

func (api *Api) TunnelExists(tunnelName string) (exists bool, tunnel cloudflare.Tunnel, err error) {
	isDeleted := false
	tunnels, _, err := api.Client.ListTunnels(
		api.Ctx,
		api.CloudflareResourceContainer,
		cloudflare.TunnelListParams{
			Name:      tunnelName,
			IsDeleted: &isDeleted,
		},
	)
	if err != nil {
		return false, cloudflare.Tunnel{}, errors.Wrap(err, "failed to get existing tunnel")
	}
	if len(tunnels) > 1 {
		return false, cloudflare.Tunnel{}, errors.Errorf("expected 1 tunnel, got %d", len(tunnels))
	}
	if len(tunnels) == 0 {
		return false, cloudflare.Tunnel{}, nil
	}
	return true, tunnels[0], nil
}

func newTunnelCreateParams(name string, secret string) cloudflare.TunnelCreateParams {
	return cloudflare.TunnelCreateParams{
		Name:      name,
		Secret:    secret,
		ConfigSrc: ConfigSourceLocal,
	}
}
