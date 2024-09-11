package cf

import (
	"fmt"
	"sort"

	"github.com/cyclingwithelephants/cloudflare-gateway-controller/internal/clients/k8s"
	"github.com/pkg/errors"
)

const (
	IngressDefaultBackend = "http_status:404"
)

var (
	IngressDefaultBackendConfig = IngressConfig{
		Hostname: "",
		Service:  IngressDefaultBackend,
	}
)

type TunnelConfigFile struct {
	TunnelId            string          `json:"tunnel"`
	CredentialsFilePath string          `json:"credentials-file"`
	Ingress             []IngressConfig `json:"ingress"`
}

type IngressConfig struct {
	Hostname string `json:"hostname,omitempty"`
	Service  string `json:"service"`
}

func NewTunnelConfigFile(tunnelId string, ingressConfig []IngressConfig) (*TunnelConfigFile, error) {
	// inject default backend config if one doesn't exist
	length := len(ingressConfig)
	if length == 0 || ingressConfig[(length-1)].Hostname != "" {
		ingressConfig = append(ingressConfig, IngressDefaultBackendConfig)
	}
	tunnelConfig := &TunnelConfigFile{
		TunnelId:            tunnelId,
		CredentialsFilePath: k8s.DeploymentCredentialFilePath,
		Ingress:             ingressConfig,
	}
	if err := tunnelConfig.validate(); err != nil {
		return &TunnelConfigFile{}, errors.Wrap(err, "failed to validate config file")
	}
	return tunnelConfig, nil
}

func (t *TunnelConfigFile) validate() error {
	if t.TunnelId == "" {
		return errors.New("TunnelId is empty")
	}
	if len(t.Ingress) == 0 {
		return errors.New("Ingress is empty")
	}
	if t.Ingress[(len(t.Ingress)-1)].Hostname != "" {
		return errors.New(fmt.Sprintf("Last ingress rule must be a catch-all with no hostname identifier e.g. `- service: %s`", IngressDefaultBackend))
	}
	return nil
}

// Sort ensures the routes are appropriately sorted for cloudflared to parse
// We have three kinds of routes, which cloudflare matches top to bottom:
// - routes with a FQDN for the Hostname field domain e.g. an.example.com (these go at the beginning)
// - routes with a wildcard value for the Hostname field e.g. *.example.com (these go after the first group)
// - a single route with no Hostname field, this is the catch-all (this must always go at the end)
func Sort(routes []IngressConfig) []IngressConfig {
	// Sort with custom logic
	sort.SliceStable(routes, func(i, j int) bool {
		// Check for the catch-all route (Hostname is empty)
		if routes[i].Hostname == "" {
			return false
		}
		if routes[j].Hostname == "" {
			return true
		}

		// Wildcards should go after fully qualified domains
		isWildcardI := routes[i].Hostname[0:2] == "*."
		isWildcardJ := routes[j].Hostname[0:2] == "*."

		if isWildcardI && !isWildcardJ {
			return false
		}
		if !isWildcardI && isWildcardJ {
			return true
		}

		// Otherwise, sort lexicographically (for FQDNs or two wildcards)
		return routes[i].Hostname < routes[j].Hostname
	})

	// Ensure catch-all route (if present) is placed at the end
	if len(routes) > 0 && routes[len(routes)-1].Hostname == "" {
		catchAll := routes[len(routes)-1]
		routes = append(routes[:len(routes)-1], catchAll)
	}

	return routes
}
