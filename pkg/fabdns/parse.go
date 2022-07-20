// Copyright 2021 FabEdge Team
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fabdns

import (
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
	// query svc in a specified cluster or not
	isAdHoc bool
}

func parseRequest(name string) (r recordRequest, err error) {
	// support the following formats:
	// {service}.{namespace}.svc.global
	// {hostname}.{cluster}.{service}.{namespace}.svc.global
	// {service}.{namespace}.{cluster}.global

	labels := dns.SplitDomainName(name)
	if len(labels) != 4 && len(labels) != 6 {
		return r, errInvalidRequest
	}

	switch {
	case len(labels) == 6:
		// hostname.cluster.service.namespace.svc.global
		r.hostname = labels[0]
		r.cluster = labels[1]
		r.service = labels[2]
		r.namespace = labels[3]
		r.isAdHoc = false
	case len(labels) == 4 && labels[2] == LabelSVC:
		// e.g. web.default.svc.global
		// If you name your cluster as "svc", it is your problem.
		r.service = labels[0]
		r.namespace = labels[1]
		r.isAdHoc = false
	case len(labels) == 4:
		// e.g. web.default.root.global
		r.service = labels[0]
		r.namespace = labels[1]
		r.cluster = labels[2]
		r.isAdHoc = true
	default:
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
	return s
}
