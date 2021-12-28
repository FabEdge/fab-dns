package dns

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/miekg/dns"
)

const defaultTTL = uint32(5)

// FabDNS implements a plugin supporting multi-cluster FabDNS spec.
type FabDNS struct {
	Next          plugin.Handler
	Zones         []string
	Fall          fall.F
	TTL           uint32
	Cluster       string
	ClusterZone   string
	ClusterRegion string
}

func New(zones []string, cluster, clusterZone, clusterRegion string) (*FabDNS, error) {

	fabdns := FabDNS{
		Zones:         zones,
		TTL:           defaultTTL,
		Cluster:       cluster,
		ClusterZone:   clusterZone,
		ClusterRegion: clusterRegion,
	}

	return &fabdns, nil
}

// ServeDNS implements the plugin.Handler interface.
func (f FabDNS) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (f FabDNS) Name() string {
	return PluginName
}

type ResponsePrinter struct {
	dns.ResponseWriter
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter) *ResponsePrinter {
	return &ResponsePrinter{ResponseWriter: w}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "example" to standard output.
func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	return r.ResponseWriter.WriteMsg(res)
}
