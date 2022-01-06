package fabdns

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Svc is the DNS schema for global services
	Svc = "svc"
	// Pod is the DNS schema for kubernetes pods
	Pod        = "pod"
	defaultTTL = uint32(5)
)

var (
	errNoItems        = errors.New("no items found")
	errInvalidRequest = errors.New("invalid query name")
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin(PluginName)

// FabDNS implements a plugin supporting multi-cluster FabDNS spec.
type FabDNS struct {
	Next          plugin.Handler
	Zones         []string
	Fall          fall.F
	TTL           uint32
	Client        client.Client
	Cluster       string
	ClusterZone   string
	ClusterRegion string
}

func New(cfg *rest.Config, zones []string, cluster, clusterZone, clusterRegion string) (*FabDNS, error) {
	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	fabdns := FabDNS{
		Zones:         zones,
		TTL:           defaultTTL,
		Client:        cli,
		Cluster:       cluster,
		ClusterZone:   clusterZone,
		ClusterRegion: clusterRegion,
	}

	return &fabdns, nil
}

// ServeDNS implements the plugin.Handler interface.
func (f FabDNS) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.QName()

	log.Debugf("Request query name is %s", qname)

	zone := plugin.Zones(f.Zones).Matches(qname)
	if len(zone) == 0 {
		log.Debugf("Request query name does not match zones %v", f.Zones)
		return f.nextOrFailure(&state, ctx, w, r, dns.RcodeNotZone, errors.New("name not contained in zone"))
	}
	zone = qname[len(qname)-len(zone):] // maintain case of original query
	state.Zone = zone

	if state.QType() != dns.TypeA && state.QType() != dns.TypeAAAA {
		log.Debugf("query type %d is not implemented", state.QType())
		return f.nextOrFailure(&state, ctx, w, r, dns.RcodeNotImplemented, fmt.Errorf("query type %d is not implemented", state.QType()))
	}

	var (
		parsedReq recordRequest
		records   []dns.RR
		err       error
	)

	parsedReq, err = parseRequest(qname, zone)
	if err != nil {
		log.Debugf("parse request err: %v", err)
		return f.nextOrFailure(&state, ctx, w, r, dns.RcodeNameError, err)
	}

	records, err = f.getRecords(&state, parsedReq)
	if f.IsNameError(err) {
		log.Debugf("get records err: %v", err)
		return f.nextOrFailure(&state, ctx, w, r, dns.RcodeNameError, err)
	}
	if err != nil {
		return dns.RcodeServerFailure, plugin.Error(f.Name(), err)
	}

	return f.writeMsg(&state, records, dns.RcodeSuccess, nil)
}

// Name implements the Handler interface.
func (f FabDNS) Name() string {
	return PluginName
}

// IsNameError returns true if err indicated a record not found condition
func (f FabDNS) IsNameError(err error) bool {
	return err == errNoItems || err == errInvalidRequest
}

func (f FabDNS) getRecords(state *request.Request, parsedReq recordRequest) ([]dns.RR, error) {
	switch state.QType() {
	case dns.TypeA:
	case dns.TypeAAAA:
	}
	return nil, nil
}

func (f FabDNS) writeMsg(state *request.Request, records []dns.RR, rcode int, err error) (int, error) {
	message := new(dns.Msg)
	message.Authoritative = true

	switch rcode {
	case dns.RcodeSuccess:
		message.SetReply(state.Req)
		message.Answer = append(message.Answer, records...)
	default:
		message.SetRcode(state.Req, rcode)
		err = plugin.Error(f.Name(), err)
	}

	state.W.WriteMsg(message)
	return rcode, err
}

func (f FabDNS) nextOrFailure(state *request.Request, ctx context.Context, w dns.ResponseWriter, r *dns.Msg, rcode int, err error) (int, error) {
	if f.Fall.Through(state.Name()) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	return f.writeMsg(state, nil, rcode, err)
}
