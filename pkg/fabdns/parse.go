package fabdns

import (
	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/miekg/dns"
)

type recordRequest struct {
	// The hostname referring to individual pod backing a headless global service.
	hostname string
	// The clustername referring to identifiers between various clusters.
	cluster string
	// The servicename referring to global service.
	service string
	// The namespace used in Kubernetes.
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen, either "pod" or "service".
	podOrSvc string
}

// parseRequest parses the qname to find all the elements we need for querying global service.
func parseRequest(name, zone string) (r recordRequest, err error) {
	// 2 Possible cases:
	// 1. (ClusterIP): service.namespace.svc.zone
	// 2. (headless endpoint): hostname.cluster.service.namespace.svc.zone

	base, _ := dnsutil.TrimZone(name, zone)
	// return NODATA for apex queries
	if base == "" || base == Svc || base == Pod {
		if base == Pod {
			return r, errInvalidRequest
		}
		return r, nil
	}
	segs := dns.SplitDomainName(base)

	// start at the right and fill out recordRequest with the bits we find, so we look for
	// svc.namespace.service and then hostname.cluster

	last := len(segs) - 1
	if last < 0 {
		return r, nil
	}
	r.podOrSvc = segs[last]
	if r.podOrSvc != Svc {
		return r, errInvalidRequest
	}
	last--
	if last < 0 {
		return r, nil
	}

	r.namespace = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	r.service = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	r.cluster = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	r.hostname = segs[last]
	last--
	if last < 0 {
		return r, nil
	}

	if last != 1 { // unrecognized labels remaining
		return r, errInvalidRequest
	}

	return r, nil
}

// String returns a string representation of r, it just returns all fields concatenated with dots.
// This is mostly used in tests.
func (r recordRequest) String() string {
	s := r.hostname
	s += "." + r.cluster
	s += "." + r.service
	s += "." + r.namespace
	s += "." + r.podOrSvc
	return s
}
