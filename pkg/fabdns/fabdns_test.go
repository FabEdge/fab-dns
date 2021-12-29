package fabdns

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	testZone            = "testzone."
	testFallthroughZone = "fall."
	testUnknownZone     = "unknown."
)

var _ = Describe("Fabdns", func() {
	Context("Implements of QType", testRequestImplements)
	Context("Fallthrough configured", testFallthroughConfigured)
	Context("ClusterIP services", testClusterIPServices)
	Context("Headless services", testHeadlessServices)
})

func testRequestImplements() {
	var (
		qname        = fmt.Sprintf("%s.%s.svc.%s", "testservice", "testns", testZone)
		testRecorder *dnstest.Recorder
	)

	fabdns := &FabDNS{
		Zones:         []string{testZone},
		Cluster:       "fabedge",
		ClusterZone:   "beijing",
		ClusterRegion: "north",
		TTL:           5,
	}

	BeforeEach(func() {
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	When("Query type is A", func() {
		It("should succeed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("Query type is AAAA", func() {
		It("should succeed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("Query type is SRV", func() {
		It("should failed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeSRV,
				Rcode: dns.RcodeNotImplemented,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("Query type is NS", func() {
		It("should failed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeNS,
				Rcode: dns.RcodeNotImplemented,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("Query type is PTR", func() {
		It("should failed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypePTR,
				Rcode: dns.RcodeNotImplemented,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})
}

func testFallthroughConfigured() {
	var (
		testRecorder *dnstest.Recorder
		fabdns       *FabDNS
	)

	BeforeEach(func() {
		fabdns = &FabDNS{
			Next:          test.NextHandler(dns.RcodeBadCookie, errors.New("fake plugin error")),
			Zones:         []string{testZone},
			Fall:          fall.F{Zones: []string{testFallthroughZone}},
			Cluster:       "fabedge",
			ClusterZone:   "beijing",
			ClusterRegion: "north",
			TTL:           5,
		}
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	When("type A DNS query for matching fallthrough zone", func() {
		It("should invoke next plugin", func() {
			qname := fmt.Sprintf("%s.%s.svc.%s", "testservice", "testns", testFallthroughZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeBadCookie,
			}
			_, err := executeTestCase(fabdns, testRecorder, testCase)
			Expect(err.Error()).Should(ContainSubstring("fake plugin"))
		})

	})

	When("type A DNS query for non-matching fallthrough zone", func() {
		It("should not invoke next plugin", func() {
			qname := fmt.Sprintf("%s.%s.svc.%s", "testservice", "testns", testUnknownZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNotZone,
			}
			_, err := executeTestCase(fabdns, testRecorder, testCase)
			Expect(err.Error()).To(ContainSubstring("not contained in zone"))
		})
	})

}

func testClusterIPServices() {
	var testRecorder *dnstest.Recorder

	fabdns := &FabDNS{
		Zones:         []string{testZone},
		Cluster:       "fabedge",
		ClusterZone:   "beijing",
		ClusterRegion: "north",
		TTL:           5,
	}

	BeforeEach(func() {
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	It("should succeed with A record response", func() {
		qname := fmt.Sprintf("%s.%s.svc.%s", "testservice", "testns", testZone)
		testCase := test.Case{
			Qname: qname,
			Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
		}
		executeTestCase(fabdns, testRecorder, testCase)
	})

	It("should succeed with AAAA record response", func() {
		qname := fmt.Sprintf("%s.%s.svc.%s", "testservice", "testns", testZone)
		testCase := test.Case{
			Qname: qname,
			Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
		}
		executeTestCase(fabdns, testRecorder, testCase)
	})

}

func testHeadlessServices() {
	var testRecorder *dnstest.Recorder

	fabdns := &FabDNS{
		Zones:         []string{testZone},
		Cluster:       "fabedge",
		ClusterZone:   "beijing",
		ClusterRegion: "north",
		TTL:           5,
	}

	BeforeEach(func() {
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	It("should succeed with A record response", func() {
		qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "hostname", "mycluster", "testservice", "testns", testZone)
		testCase := test.Case{
			Qname: qname,
			Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
		}
		executeTestCase(fabdns, testRecorder, testCase)
	})

	It("should succeed with AAAA record response", func() {
		qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "hostname", "mycluster", "testservice", "testns", testZone)
		testCase := test.Case{
			Qname: qname,
			Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
		}
		executeTestCase(fabdns, testRecorder, testCase)
	})
}

func executeTestCase(fabdns *FabDNS, recorder *dnstest.Recorder, testcase test.Case) (rcode int, err error) {
	rcode, err = fabdns.ServeDNS(context.TODO(), recorder, testcase.Msg())
	Expect(rcode).To(Equal(testcase.Rcode))

	if testcase.Rcode == dns.RcodeSuccess {
		Expect(err).To(Succeed())
		Expect(rcode).To(Equal(dns.RcodeSuccess))
	} else {
		Expect(err).To(HaveOccurred())
	}
	return rcode, err
}
