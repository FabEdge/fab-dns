package fabdns

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

const PluginName = "fabdns"

// Hook for unit tests
var buildConfigFromFlags = clientcmd.BuildConfigFromFlags

func init() {
	caddy.RegisterPlugin(PluginName, caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
	_ = apis.AddToScheme(scheme.Scheme)
}

func setup(c *caddy.Controller) error {

	fabdns, err := fabdnsParse(c)
	if err != nil {
		return plugin.Error(PluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		fabdns.Next = next
		return fabdns
	})

	return nil
}

func fabdnsParse(c *caddy.Controller) (*FabDNS, error) {

	c.Next() // Skip "fabdns" label

	zones := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
	var (
		fabFall       fall.F
		ttl           int
		masterurl     string
		kubeconfig    string
		cluster       string
		clusterZone   string
		clusterRegion string
	)
	for c.NextBlock() {
		switch c.Val() {
		case "fallthrough":
			fabFall.SetZonesFromArgs(c.RemainingArgs())
		case "kubeconfig":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			kubeconfig = args[0]
		case "masterurl":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			masterurl = args[0]
		case "cluster":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			cluster = args[0]
		case "zone":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			clusterZone = args[0]
		case "region":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			clusterRegion = args[0]
		case "ttl":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			var err error
			ttl, err = strconv.Atoi(args[0])
			if err != nil {
				return nil, c.Errf("ttl %v", err)
			}
			if ttl <= 0 || ttl > 3600 {
				return nil, c.Errf("ttl %d is out of range (0, 3600], default ttl is %d if not configured", ttl, defaultTTL)
			}
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}
	if ttl == 0 {
		ttl = defaultTTL
	}

	cfg, err := buildConfigFromFlags(masterurl, kubeconfig)
	if err != nil {
		return nil, err
	}

	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	fabdns := &FabDNS{
		Zones:  zones,
		Fall:   fabFall,
		TTL:    uint32(ttl),
		Client: cli,
		Cluster: ClusterInfo{
			Name:   cluster,
			Zone:   clusterZone,
			Region: clusterRegion,
		},
	}

	return fabdns, nil
}
