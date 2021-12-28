package dns

import (
	"flag"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

const PluginName = "fabdns"

var (
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

func init() {
	flag.StringVar(&cluster, "cluster", "", "Cluster name. Required for recognizing cluster.")
	flag.StringVar(&clusterZone, "cluster-zone", "", "Cluster zone. Required for recognizing zone.")
	flag.StringVar(&clusterRegion, "cluster-region", "", "Cluster region. Required for recognizing region.")
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

	fabdns, err := New(zones, cluster, clusterZone, clusterRegion)
	if err != nil {
		return nil, err
	}

	for c.NextBlock() {
		switch c.Val() {
		case "fallthrough":
			fabdns.Fall.SetZonesFromArgs(c.RemainingArgs())
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}

	return fabdns, nil
}
