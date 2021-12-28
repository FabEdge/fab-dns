package dns

import (
	"context"
	"fmt"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fabdns", func() {
	var (
		recorder *dnstest.Recorder
	)
	fabdns := &FabDNS{
		Zones:         []string{"svc.global."},
		Cluster:       "fabedge",
		ClusterZone:   "beijing",
		ClusterRegion: "north",
		TTL:           5,
	}

	BeforeEach(func() {
		recorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	It("should succeed with A record response", func() {
		qname := fmt.Sprintf("%s.%s.svc.global", "testservice", "testns")
		testCase := test.Case{
			Qname: qname,
			Qtype: dns.TypeA,
		}
		rcode, err := fabdns.ServeDNS(context.TODO(), recorder, testCase.Msg())
		Expect(err).To(Succeed())
		Expect(rcode).To(Equal(dns.RcodeSuccess))
	})
})
