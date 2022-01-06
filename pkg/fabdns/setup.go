package fabdns

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"k8s.io/client-go/tools/clientcmd"
)

const PluginName = "fabdns"

// Hook for unit tests
var buildConfigFromFlags = clientcmd.BuildConfigFromFlags

var (
	masterurl     string
	kubeconfig    string
	cluster       string
	clusterZone   string
	clusterRegion string
)

func init() {
	caddy.RegisterPlugin(PluginName, caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
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
		fabFall fall.F
		ttl     uint32
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
		case "cluster-zone":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			clusterZone = args[0]
		case "cluster-region":
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
			t, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, c.Err(err.Error())
			}
			if t < 0 || t > 3600 {
				return nil, c.Errf("ttl must be in range [0, 3600] configured %d", t)
			}
			ttl = uint32(t)
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}

	cfg, err := buildConfigFromFlags(masterurl, kubeconfig)
	if err != nil {
		return nil, err
	}

	fabdns, err := New(cfg, zones, cluster, clusterZone, clusterRegion)
	if err != nil {
		return nil, err
	}
	fabdns.Fall = fabFall
	if ttl > 0 {
		fabdns.TTL = ttl
	}

	return fabdns, nil
}
