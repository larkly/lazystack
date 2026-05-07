package cloud

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/shared"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
)

// resolveMicroversion determines which Nova microversion to use.
// It checks OS_COMPUTE_API_VERSION first (user override), then falls back
// to runtime negotiation. Returns (maxVersion, usedVersion, degradationWarning).
func resolveMicroversion(ctx context.Context, compute *gophercloud.ServiceClient) (string, string, string) {
	// Check for user-specified microversion via environment variable
	if userVersion := os.Getenv("OS_COMPUTE_API_VERSION"); userVersion != "" {
		// Validate microversion format (X.Y or X.Y.Z)
		if _, _, err := parseMicroversion(userVersion); err != nil {
			shared.Debugf("[cloud] resolveMicroversion: invalid OS_COMPUTE_API_VERSION=%q: %v, ignoring", userVersion, err)
		} else {
			ceiling := "2.100"
			used := minMicroversion(userVersion, ceiling)
			if used != userVersion {
				shared.Debugf("[cloud] resolveMicroversion: user requested %s, capped at %s", userVersion, used)
			}
			shared.Debugf("[cloud] resolveMicroversion: using user-specified microversion %s", used)
			return "user-specified", used, ""
		}
	}
	return negotiateNovaMicroversion(ctx, compute)
}
// negotiateNovaMicroversion queries the Nova API for the max supported microversion,
// caps at 2.100 (our known-good ceiling), and returns the negotiated version.
// Returns (maxVersion, usedVersion, degradationWarning).
func negotiateNovaMicroversion(ctx context.Context, compute *gophercloud.ServiceClient) (string, string, string) {
	const ceiling = "2.100"

	// Save current microversion and set to base for version discovery
	// Version discovery returns the API versions supported by the deployment.
	url := compute.ServiceURL("")
	resp, err := compute.Get(ctx, url, nil, &gophercloud.RequestOpts{
		// Version discovery does not need a microversion
	})
	if err != nil {
		shared.Debugf("[cloud] negotiateMicroversion: version discovery failed: %v, falling back to %s", err, ceiling)
		return "unknown", ceiling, ""
	}

	var versionDoc struct {
		Versions []struct {
			ID         string `json:"id"`
			Version    string `json:"version"`
			MinVersion string `json:"min_version"`
		} `json:"versions"`
		Version struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"version"`
	}

	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&versionDoc); err != nil {
		shared.Debugf("[cloud] negotiateMicroversion: JSON parse failed: %v, falling back to %s", err, ceiling)
		return "unknown", ceiling, ""
	}

	// Parse the latest supported version from the response
	// Nova version document has versions array with IDs like "v2.1"
	// "version" field = max supported microversion, "min_version" = minimum
	maxVersion := "unknown"
	for _, v := range versionDoc.Versions {
		if v.ID == "v2.1" {
			maxVersion = v.Version
			break
		}
	}

	// If we couldn't parse it, fall back
	if maxVersion == "" || maxVersion == "unknown" {
		shared.Debugf("[cloud] negotiateMicroversion: couldn't determine max version, using %s", ceiling)
		return "unknown", ceiling, ""
	}

	// Compare: use the lower of maxVersion and ceiling (2.100)
	usedVersion := minMicroversion(maxVersion, ceiling)
	degradeWarning := ""
	if usedVersion != ceiling {
		degradeWarning = fmt.Sprintf(
			"Nova microversion %s (max supported: %s, requested: %s). Some features may be limited."+
				" Upgrade to OpenStack Zed (2023.1) or later for full functionality.",
			usedVersion, maxVersion, ceiling,
		)
		shared.Debugf("[cloud] negotiateMicroversion: degraded: %s", degradeWarning)
	}

	return maxVersion, usedVersion, degradeWarning
}

// minMicroversion returns the numerically lower of two microversion strings.
// Microversions are formatted as "X.Y" — compare major then minor.
func minMicroversion(a, b string) string {
	ma, mi, _ := parseMicroversion(a)
	mb, mj, _ := parseMicroversion(b)

	if ma > mb || (ma == mb && mi > mj) {
		return b
	}
	return a
}

func parseMicroversion(v string) (int, int, error) {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid microversion %q: expected format X.Y or X.Y.Z", v)
	}
	ma, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid microversion major %q: %w", parts[0], err)
	}
	mi, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid microversion minor %q: %w", parts[1], err)
	}
	return ma, mi, nil
}

// Client holds authenticated OpenStack service clients.
type Client struct {
	CloudName            string
	Region               string
	Compute              *gophercloud.ServiceClient
	Image                *gophercloud.ServiceClient
	Network              *gophercloud.ServiceClient
	BlockStorage         *gophercloud.ServiceClient
	LoadBalancer         *gophercloud.ServiceClient
	ProviderClient       *gophercloud.ProviderClient
	EndpointOpts         gophercloud.EndpointOpts
	NovaMicroversionMax  string // max supported by this deployment
	NovaMicroversionUsed string // actual microversion in use (≤ 2.100)
}

// Connect authenticates to the given cloud and initializes service clients.
func Connect(ctx context.Context, cloudName string) (*Client, error) {
	shared.Debugf("[cloud] Connect: starting, cloud=%s", cloudName)
	ao, eo, tlsConfig, err := clouds.Parse(clouds.WithCloudName(cloudName), clouds.WithLocations(CloudsYamlPaths()...))
	if err != nil {
		shared.Debugf("[cloud] Connect: error parsing cloud config: %v", err)
		return nil, fmt.Errorf("parsing cloud %q: %w", cloudName, err)
	}
	return connectWithOpts(ctx, ao, eo, tlsConfig, cloudName)
}

// ConnectWithProject authenticates scoped to a specific project.
func ConnectWithProject(ctx context.Context, cloudName, projectID string) (*Client, error) {
	shared.Debugf("[cloud] ConnectWithProject: starting, cloud=%s projectID=%s", cloudName, projectID)
	ao, eo, tlsConfig, err := clouds.Parse(clouds.WithCloudName(cloudName), clouds.WithLocations(CloudsYamlPaths()...))
	if err != nil {
		shared.Debugf("[cloud] ConnectWithProject: error parsing cloud config: %v", err)
		return nil, fmt.Errorf("parsing cloud %q: %w", cloudName, err)
	}
	ao.TenantID = projectID
	ao.TenantName = "" // Clear TenantName to avoid conflicts
	return connectWithOpts(ctx, ao, eo, tlsConfig, cloudName)
}

func connectWithOpts(ctx context.Context, ao gophercloud.AuthOptions, eo gophercloud.EndpointOpts, tlsConfig *tls.Config, cloudName string) (*Client, error) {
	shared.Debugf("[cloud] connectWithOpts: authenticating to %s", cloudName)
	providerClient, err := config.NewProviderClient(ctx, ao, config.WithTLSConfig(tlsConfig))
	if err != nil {
		shared.Debugf("[cloud] connectWithOpts: authentication error: %v", err)
		return nil, fmt.Errorf("authenticating to %q: %w", cloudName, err)
	}

	shared.Debugf("[cloud] connectWithOpts: creating compute client")
	compute, err := openstack.NewComputeV2(providerClient, eo)
	if err != nil {
		shared.Debugf("[cloud] connectWithOpts: compute client error: %v", err)
		return nil, fmt.Errorf("compute client: %w", err)
	}
	compute.Microversion = "2.100"

	// Resolve the Nova microversion: check user override first, then negotiate.
	maxVersion, usedVersion, degradeWarning := resolveMicroversion(ctx, compute)
	compute.Microversion = usedVersion

	shared.Debugf("[cloud] connectWithOpts: creating image client")
	image, err := openstack.NewImageV2(providerClient, eo)
	if err != nil {
		shared.Debugf("[cloud] connectWithOpts: image client error: %v", err)
		return nil, fmt.Errorf("image client: %w", err)
	}

	shared.Debugf("[cloud] connectWithOpts: creating network client")
	network, err := openstack.NewNetworkV2(providerClient, eo)
	if err != nil {
		shared.Debugf("[cloud] connectWithOpts: network client error: %v", err)
		return nil, fmt.Errorf("network client: %w", err)
	}

	// BlockStorage — try v3 first ("block-storage"), then v2, then v1 ("volume")
	// Different clouds register Cinder under different service types
	shared.Debugf("[cloud] connectWithOpts: creating block storage client")
	blockStorage := tryBlockStorage(providerClient, eo)
	if blockStorage == nil {
		shared.Debugf("[cloud] connectWithOpts: block storage client unavailable")
	}

	// LoadBalancer (Octavia) — optional service
	shared.Debugf("[cloud] connectWithOpts: creating load balancer client")
	loadBalancer := tryLoadBalancer(providerClient, eo)
	if loadBalancer == nil {
		shared.Debugf("[cloud] connectWithOpts: load balancer client unavailable")
	}

	region := eo.Region
	if region == "" {
		region = "default"
	}

	shared.Debugf("[cloud] connectWithOpts: success, cloud=%s region=%s nova_version=%s (max=%s)", cloudName, region, usedVersion, maxVersion)
	if degradeWarning != "" {
		shared.Debugf("[cloud] connectWithOpts: degradation warning: %s", degradeWarning)
	}
	return &Client{
		CloudName:            cloudName,
		Region:               region,
		Compute:              compute,
		Image:                image,
		Network:              network,
		BlockStorage:         blockStorage,
		LoadBalancer:         loadBalancer,
		ProviderClient:       providerClient,
		EndpointOpts:         eo,
		NovaMicroversionMax:  maxVersion,
		NovaMicroversionUsed: usedVersion,
	}, nil
}

func tryLoadBalancer(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) *gophercloud.ServiceClient {
	if sc, err := openstack.NewLoadBalancerV2(pc, eo); err == nil {
		return sc
	}
	return nil
}

func tryBlockStorage(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) *gophercloud.ServiceClient {
	// Try the standard v3 service type
	if sc, err := openstack.NewBlockStorageV3(pc, eo); err == nil {
		return sc
	}
	// Try v2
	if sc, err := openstack.NewBlockStorageV2(pc, eo); err == nil {
		return sc
	}
	// Try v1 (service type "volume")
	if sc, err := openstack.NewBlockStorageV1(pc, eo); err == nil {
		return sc
	}
	return nil
}
