package fabdns

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

const (
	// Svc is the DNS schema for global services
	Svc = "svc"
	// Pod is the DNS schema for kubernetes pods
	Pod        = "pod"
	defaultTTL = 5
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

func (f *FabDNS) getRecords(state *request.Request, parsedReq recordRequest) ([]dns.RR, error) {
	namespace, service, clustername, hostname := parsedReq.namespace, parsedReq.service, parsedReq.cluster, parsedReq.hostname

	if len(namespace) == 0 || len(service) == 0 {
		return nil, errNoItems
	}
	if len(hostname) > 0 && clustername == "" {
		return nil, errInvalidRequest
	}

	var (
		globalService apis.GlobalService
		serviceKey    = client.ObjectKey{
			Namespace: namespace,
			Name:      service,
		}
	)

	err := f.Client.Get(context.TODO(), serviceKey, &globalService)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errNoItems
		}
		log.Errorf("failed to find GlobalService err: %v, query name is %s", err, state.Name())
		return nil, err
	}

	var (
		headless              = clustername != ""
		clusterMatchedRecords []dns.RR
		inZoneRecords         []dns.RR
		inRegionRecords       []dns.RR
	)
	if headless {
		if globalService.Spec.Type != apis.Headless {
			log.Debugf("the type of GlobalService is %s not match with %s", globalService.Spec.Type, apis.Headless)
			return nil, errInvalidRequest
		}
	} else {
		// local cluster endpoints preference
		clustername = f.Cluster
	}
	for _, endpoint := range globalService.Spec.Endpoints {
		switch {
		case endpoint.Cluster == clustername:
			if headless {
				if endpoint.Hostname != nil && *endpoint.Hostname == hostname {
					clusterMatchedRecords = append(clusterMatchedRecords, f.generateRecords(state, endpoint)...)
				}
				continue
			}
			clusterMatchedRecords = append(clusterMatchedRecords, f.generateRecords(state, endpoint)...)

		case endpoint.Zone == f.ClusterZone:
			// in zone
			inZoneRecords = append(inZoneRecords, f.generateRecords(state, endpoint)...)

		case endpoint.Region == f.ClusterRegion:
			// in region
			inRegionRecords = append(inRegionRecords, f.generateRecords(state, endpoint)...)
		}
	}

	if headless {
		if len(clusterMatchedRecords) == 0 {
			return nil, errNoItems
		}
		return clusterMatchedRecords, nil
	}

	switch {
	case len(clusterMatchedRecords) > 0:
		return clusterMatchedRecords, nil
	case len(inZoneRecords) > 0:
		return inZoneRecords, nil
	case len(inRegionRecords) > 0:
		return inRegionRecords, nil
	default:
		allRecords := make([]dns.RR, 0)
		for _, endpoint := range globalService.Spec.Endpoints {
			allRecords = append(allRecords, f.generateRecords(state, endpoint)...)
		}
		if len(allRecords) == 0 {
			return nil, errNoItems
		}
		return allRecords, nil
	}
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

func (f FabDNS) generateRecords(state *request.Request, endpoint apis.Endpoint) (records []dns.RR) {
	switch state.QType() {
	case dns.TypeA:
		for _, addr := range endpoint.Addresses {
			if ip, ok := verifyIP(addr); ok {
				if isIPv4(ip) {
					records = append(records, &dns.A{
						Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: state.QClass(), Ttl: f.TTL},
						A:   ip.To4(),
					})
				}
			}
		}
	case dns.TypeAAAA:
		for _, addr := range endpoint.Addresses {
			if ip, ok := verifyIP(addr); ok {
				if !isIPv4(ip) {
					records = append(records, &dns.AAAA{
						Hdr:  dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: state.QClass(), Ttl: f.TTL},
						AAAA: ip.To16(),
					})
				}
			}
		}
	}
	return
}

func verifyIP(address string) (net.IP, bool) {
	ip := net.ParseIP(address)
	return ip, ip != nil
}

func isIPv4(ip net.IP) bool {
	return ip.To4() != nil
}
