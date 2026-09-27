package cloud

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/larkly/lazystack/internal/shared"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
)

const (
	// novaCeilingMicroversion is the newest Nova microversion lazystack is
	// known to work with.
	novaCeilingMicroversion = "2.100"
	// novaBaseMicroversion is the first microversion of the v2.1 API, which
	// every Nova that serves v2.1 accepts. It is used whenever the supported
	// range cannot be determined.
	novaBaseMicroversion = "2.1"
)

// resolveMicroversion determines which Nova microversion to use.
// It checks OS_COMPUTE_API_VERSION first (user override), then falls back
// to runtime negotiation. Returns (maxVersion, usedVersion, degradationWarning).
//
// The override must be a plain Nova microversion "X.Y" of at least 2.1: no
// whitespace, sign, "v" prefix, leading zeros or third component. Anything
// else (including "latest") is ignored with a warning and the version is
// negotiated instead; nothing is silently truncated or normalised. A valid
// override above the ceiling is capped at 2.100.
func resolveMicroversion(ctx context.Context, compute *gophercloud.ServiceClient) (string, string, string) {
	userVersion := os.Getenv("OS_COMPUTE_API_VERSION")
	if userVersion == "" {
		return negotiateNovaMicroversion(ctx, compute)
	}
	if err := validateNovaMicroversion(userVersion); err != nil {
		shared.Debugf("[cloud] resolveMicroversion: invalid OS_COMPUTE_API_VERSION=%q: %v, ignoring", userVersion, err)
		maxVersion, used, warning := negotiateNovaMicroversion(ctx, compute)
		ignored := fmt.Sprintf("Ignoring OS_COMPUTE_API_VERSION=%q (expected a Nova microversion such as 2.60).", userVersion)
		if warning != "" {
			ignored += " " + warning
		}
		return maxVersion, used, ignored
	}
	used := minMicroversion(userVersion, novaCeilingMicroversion)
	if used != userVersion {
		shared.Debugf("[cloud] resolveMicroversion: user requested %s, capped at %s", userVersion, used)
	}
	shared.Debugf("[cloud] resolveMicroversion: using user-specified microversion %s", used)
	return "user-specified", used, ""
}

// negotiateNovaMicroversion queries the Nova API for the max supported microversion,
// caps at 2.100 (our known-good ceiling), and returns the negotiated version.
// Returns (maxVersion, usedVersion, degradationWarning).
//
// If discovery fails (HTTP error, undecodable body, missing or malformed
// version) the base microversion 2.1 is used, since assuming the ceiling
// would make every later request fail on an older Nova, and a warning is
// returned so the degradation is visible.
func negotiateNovaMicroversion(ctx context.Context, compute *gophercloud.ServiceClient) (string, string, string) {
	const ceiling = novaCeilingMicroversion
	fallback := func(reason string) (string, string, string) {
		shared.Debugf("[cloud] negotiateMicroversion: %s, falling back to %s", reason, novaBaseMicroversion)
		return "unknown", novaBaseMicroversion, fmt.Sprintf(
			"Nova version discovery failed; using base microversion %s. Some features may be unavailable.",
			novaBaseMicroversion)
	}

	// Version discovery returns the API versions supported by the deployment.
	url := compute.ServiceURL("")
	// 300 Multiple Choices is served by unversioned compute endpoints and
	// carries the same versions document as a 200.
	resp, err := compute.Get(ctx, url, nil, &gophercloud.RequestOpts{
		OkCodes:          []int{200, 300},
		KeepResponseBody: true,
	})
	if err != nil {
		return fallback(fmt.Sprintf("version discovery failed: %v", err))
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
		return fallback(fmt.Sprintf("JSON parse failed: %v", err))
	}

	// Parse the latest supported version from the response
	// Nova version document has versions array with IDs like "v2.1"
	// "version" field = max supported microversion, "min_version" = minimum
	maxVersion := ""
	for _, v := range versionDoc.Versions {
		if v.ID == "v2.1" {
			maxVersion = v.Version
			break
		}
	}
	if maxVersion == "" {
		maxVersion = versionDoc.Version.Version
	}
	if maxVersion == "" {
		return fallback("couldn't determine max version")
	}
	if err := validateNovaMicroversion(maxVersion); err != nil {
		return fallback(fmt.Sprintf("discovered version unusable: %v", err))
	}

	// Compare: use the lower of maxVersion and ceiling (2.100)
	usedVersion := minMicroversion(maxVersion, ceiling)
	degradeWarning := ""
	if usedVersion != ceiling {
		degradeWarning = fmt.Sprintf(
			"Nova microversion %s (max supported: %s, requested: %s). Some features may be limited.",
			usedVersion, maxVersion, ceiling,
		)
		shared.Debugf("[cloud] negotiateMicroversion: degraded: %s", degradeWarning)
	}

	return maxVersion, usedVersion, degradeWarning
}

// minMicroversion returns the numerically lower of two microversion strings.
// Both must already be valid (see parseMicroversion); callers validate first.
func minMicroversion(a, b string) string {
	ma, mi, _ := parseMicroversion(a)
	mb, mj, _ := parseMicroversion(b)

	if ma > mb || (ma == mb && mi > mj) {
		return b
	}
	return a
}

// microversionPattern matches Nova's own microversion syntax: "X.Y" with no
// leading zeros, sign, whitespace, prefix or extra components.
var microversionPattern = regexp.MustCompile(`^([1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func parseMicroversion(v string) (int, int, error) {
	m := microversionPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, fmt.Errorf("invalid microversion %q: expected format X.Y", v)
	}
	ma, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid microversion major %q: %w", m[1], err)
	}
	mi, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid microversion minor %q: %w", m[2], err)
	}
	return ma, mi, nil
}

// validateNovaMicroversion reports whether v is a well-formed microversion
// that can be sent to Nova's v2.1 API, i.e. at least 2.1.
func validateNovaMicroversion(v string) error {
	if _, _, err := parseMicroversion(v); err != nil {
		return err
	}
	if minMicroversion(v, novaBaseMicroversion) != novaBaseMicroversion || v == "2.0" {
		return fmt.Errorf("microversion %q is below %s", v, novaBaseMicroversion)
	}
	return nil
}

// Client holds authenticated OpenStack service clients.
type Client struct {
	CloudName            string
	Region               string // configured region, else the selected compute endpoint's region, else "auto"
	Compute              *gophercloud.ServiceClient
	Image                *gophercloud.ServiceClient
	Network              *gophercloud.ServiceClient
	BlockStorage         *gophercloud.ServiceClient
	LoadBalancer         *gophercloud.ServiceClient
	DNS                  *gophercloud.ServiceClient
	ProviderClient       *gophercloud.ProviderClient
	EndpointOpts         gophercloud.EndpointOpts
	NovaMicroversionMax  string // max supported by this deployment
	NovaMicroversionUsed string // actual microversion in use (≤ 2.100)
	// CapabilityWarning describes reduced functionality (older Nova, failed
	// version discovery, ignored override). Empty when fully capable. It
	// never contains credentials, tokens or endpoint URLs.
	CapabilityWarning string
}

// resolveRegion returns the region to display. A configured region is used
// as-is. Otherwise the SDK picked the first matching catalog endpoint, so the
// region of the compute endpoint actually in use is looked up in the token's
// catalog. "auto" is returned when that cannot be determined.
func resolveRegion(pc *gophercloud.ProviderClient, region, computeURL string) string {
	if region != "" {
		return region
	}
	if pc == nil {
		return "auto"
	}
	result, ok := pc.GetAuthResult().(interface {
		ExtractServiceCatalog() (*tokens.ServiceCatalog, error)
	})
	if !ok {
		return "auto"
	}
	catalog, err := result.ExtractServiceCatalog()
	if err != nil || catalog == nil {
		return "auto"
	}
	for _, entry := range catalog.Entries {
		if entry.Type != "compute" {
			continue
		}
		for _, ep := range entry.Endpoints {
			if gophercloud.NormalizeURL(ep.URL) != computeURL {
				continue
			}
			if ep.Region != "" {
				return ep.Region
			}
			if ep.RegionID != "" {
				return ep.RegionID
			}
		}
	}
	return "auto"
}

// parseCloud reads the named cloud's settings from the clouds.yaml chosen by
// selectCloudsYaml. Passing that single file (rather than the whole search
// list) keeps gophercloud from picking a different file than the one the
// cloud list came from, and pairs secure.yaml with the selected directory.
func parseCloud(cloudName string) (gophercloud.AuthOptions, gophercloud.EndpointOpts, *tls.Config, error) {
	path, _, err := selectCloudsYaml()
	if err != nil {
		return gophercloud.AuthOptions{}, gophercloud.EndpointOpts{}, nil, err
	}
	return clouds.Parse(clouds.WithCloudName(cloudName), clouds.WithLocations(path))
}

// Connect authenticates to the given cloud and initializes service clients.
func Connect(ctx context.Context, cloudName string) (*Client, error) {
	shared.Debugf("[cloud] Connect: starting, cloud=%s", cloudName)
	ao, eo, tlsConfig, err := parseCloud(cloudName)
	if err != nil {
		shared.Debugf("[cloud] Connect: error parsing cloud config: %v", err)
		return nil, fmt.Errorf("parsing cloud %q: %w", cloudName, err)
	}
	return connectWithOpts(ctx, ao, eo, tlsConfig, cloudName)
}

// ConnectWithProject authenticates scoped to a specific project.
func ConnectWithProject(ctx context.Context, cloudName, projectID string) (*Client, error) {
	shared.Debugf("[cloud] ConnectWithProject: starting, cloud=%s projectID=%s", cloudName, projectID)
	ao, eo, tlsConfig, err := parseCloud(cloudName)
	if err != nil {
		shared.Debugf("[cloud] ConnectWithProject: error parsing cloud config: %v", err)
		return nil, fmt.Errorf("parsing cloud %q: %w", cloudName, err)
	}
	// Token- and application-credential-based auth cannot be re-scoped: the
	// token is issued for a fixed scope (app creds are bound to their owning
	// project) and there is no way to exchange it for a project-scoped one.
	if ao.TokenID != "" || ao.ApplicationCredentialID != "" || ao.ApplicationCredentialName != "" {
		return nil, fmt.Errorf("project switch to %s for cloud %q: re-scoping is not possible with token or application-credential auth; use password auth", projectID, cloudName)
	}
	// Drop any scope populated from clouds.yaml (e.g. trust_id) so the token
	// ends up scoped to the target project instead.
	ao.Scope = nil
	ao.TenantID = projectID
	ao.TenantName = "" // Clear TenantName to avoid conflicts
	return connectWithOpts(ctx, ao, eo, tlsConfig, cloudName)
}

func connectWithOpts(ctx context.Context, ao gophercloud.AuthOptions, eo gophercloud.EndpointOpts, tlsConfig *tls.Config, cloudName string) (*Client, error) {
	shared.Debugf("[cloud] connectWithOpts: authenticating to %s", cloudName)
	// TLS config is applied inside newHTTPClient: gophercloud's
	// config.WithTLSConfig replaces the transport of any client passed via
	// config.WithHTTPClient, so both must be combined into one option.
	providerClient, err := config.NewProviderClient(ctx, ao, config.WithHTTPClient(newHTTPClient(tlsConfig)))
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
	// Resolve the Nova microversion: check user override first, then negotiate.
	// Note: do NOT pre-set compute.Microversion before negotiation — the discovery
	// request itself must not include an X-OpenStack-Nova-API-Version header,
	// or a server whose max is lower than our ceiling (e.g. 2.88) will reject it
	// with 406 before we can read its supported range.
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

	// DNS (Designate) — optional service
	shared.Debugf("[cloud] connectWithOpts: creating DNS client")
	dns := tryDNS(providerClient, eo)
	if dns == nil {
		shared.Debugf("[cloud] connectWithOpts: DNS client unavailable")
	}

	region := resolveRegion(providerClient, eo.Region, compute.Endpoint)

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
		DNS:                  dns,
		ProviderClient:       providerClient,
		EndpointOpts:         eo,
		NovaMicroversionMax:  maxVersion,
		NovaMicroversionUsed: usedVersion,
		CapabilityWarning:    degradeWarning,
	}, nil
}

func tryLoadBalancer(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) *gophercloud.ServiceClient {
	if sc, err := openstack.NewLoadBalancerV2(pc, eo); err == nil {
		return sc
	}
	return nil
}

func tryDNS(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) *gophercloud.ServiceClient {
	if sc, err := openstack.NewDNSV2(pc, eo); err == nil {
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
