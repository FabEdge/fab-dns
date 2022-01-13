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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

const (
	testZone            = "testzone."
	testFallthroughZone = "fall."
	testUnknownZone     = "unknown."

	serviceNginx      = "nginx"
	namespaceDefault  = "default"
	testLocalCluster  = "beijing"
	testClusterZone   = "beijing"
	testClusterRegion = "north"
)

var _ = Describe("Fabdns", func() {
	Context("Implements of QType", testRequestImplements)
	Context("Fallthrough configured", testFallthroughConfigured)
	Context("ClusterIP services", testClusterIPServices)
	Context("Headless services", testHeadlessServices)
})

func testRequestImplements() {
	var (
		qname        = fmt.Sprintf("%s.%s.svc.%s", serviceNginx, namespaceDefault, testZone)
		testService  apis.GlobalService
		testRecorder *dnstest.Recorder
		fabdns       *FabDNS
	)

	BeforeEach(func() {
		fabdns = &FabDNS{
			Zones:         []string{testZone},
			TTL:           5,
			Client:        testK8sClient,
			Cluster:       testLocalCluster,
			ClusterZone:   testClusterZone,
			ClusterRegion: testClusterRegion,
		}
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})

		testService = apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        serviceNginx,
				Namespace:   namespaceDefault,
				ClusterName: testLocalCluster,
			},
			Spec: apis.GlobalServiceSpec{
				Type: apis.ClusterIP,
				Ports: []apis.ServicePort{
					{
						Port:     80,
						Name:     "web",
						Protocol: corev1.ProtocolTCP,
					},
				},
				Endpoints: []apis.Endpoint{
					{
						Cluster:   testLocalCluster,
						Region:    testClusterRegion,
						Zone:      testClusterZone,
						Addresses: []string{"192.168.1.1"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      serviceNginx,
							Namespace: namespaceDefault,
						},
					},
					{
						Cluster:   testLocalCluster,
						Region:    testClusterRegion,
						Zone:      testClusterZone,
						Addresses: []string{"192.168.1.2"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      serviceNginx,
							Namespace: namespaceDefault,
						},
					},
					{
						Cluster:   testLocalCluster,
						Region:    testClusterRegion,
						Zone:      testClusterZone,
						Addresses: []string{"192.168.1.3", "FF01::3"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      serviceNginx,
							Namespace: namespaceDefault,
						},
					},
				},
			},
		}
		createGlobalService(testK8sClient, &testService)
	})

	AfterEach(func() {
		deleteGlobalService(testK8sClient, &testService)
	})

	When("Query type is A", func() {
		It("should succeed", func() {
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.1")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.2")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.3")),
				},
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
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
				},
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
	var (
		svcNginxNorth = "nginx-north"
		testservice   apis.GlobalService

		testRecorder *dnstest.Recorder
		fabdns       *FabDNS
	)

	BeforeEach(func() {
		testservice = apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcNginxNorth,
				Namespace:   namespaceDefault,
				ClusterName: "haidian",
			},
			Spec: apis.GlobalServiceSpec{
				Type: apis.ClusterIP,
				Ports: []apis.ServicePort{
					{
						Port:     80,
						Name:     "web",
						Protocol: corev1.ProtocolTCP,
					},
				},
				Endpoints: []apis.Endpoint{
					{
						Cluster:   "xicheng",
						Region:    "north",
						Zone:      "beijing",
						Addresses: []string{"192.168.1.1"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Cluster:   "minhang",
						Region:    "south",
						Zone:      "shanghai",
						Addresses: []string{"192.168.1.2", "FF01::2"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Cluster:   "chaoyang",
						Region:    "north",
						Zone:      "beijing",
						Addresses: []string{"192.168.1.3", "FF01::3"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Cluster:   "shijiazhuang",
						Region:    "north",
						Zone:      "hebei",
						Addresses: []string{"192.168.1.4", "FF01::4"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
				},
			},
		}
		createGlobalService(testK8sClient, &testservice)

		fabdns = &FabDNS{
			Zones:         []string{testZone},
			TTL:           5,
			Client:        testK8sClient,
			Cluster:       testLocalCluster,
			ClusterZone:   testClusterZone,
			ClusterRegion: testClusterRegion,
		}
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	AfterEach(func() {
		deleteGlobalService(testK8sClient, &testservice)
	})

	When("global service type of ClusterIP exists", func() {

		It("should succeed with same cluster A record response", func() {
			fabdns.Cluster = "chaoyang"
			fabdns.ClusterZone = "beijing"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.3")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with same cluster zone A record response", func() {
			fabdns.Cluster = "haidian"
			fabdns.ClusterZone = "beijing"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.1")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.3")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with same cluster region A record response", func() {
			fabdns.Cluster = "tianjin"
			fabdns.ClusterZone = "tianjin"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.1")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.3")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.4")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with all A record response", func() {
			fabdns.Cluster = "xian"
			fabdns.ClusterZone = "shanxi"
			fabdns.ClusterRegion = "west"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.1")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.2")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.3")),
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.4")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with same cluster AAAA record response", func() {
			fabdns.Cluster = "chaoyang"
			fabdns.ClusterZone = "beijing"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with same zone AAAA record response", func() {
			fabdns.Cluster = "xicheng"
			fabdns.ClusterZone = "beijing"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with same region AAAA record response", func() {
			fabdns.Cluster = "tianjin"
			fabdns.ClusterZone = "tianjin"
			fabdns.ClusterRegion = "north"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::4")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with all AAAA record response", func() {
			fabdns.Cluster = "xian"
			fabdns.ClusterZone = "shanxi"
			fabdns.ClusterRegion = "west"

			qname := fmt.Sprintf("%s.%s.svc.%s", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::2")),
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::4")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

	})

	When("global service type of ClusterIP exists but request is Headless", func() {
		It("should failed with A request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "testhostname", "testcluster", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should failed with AAAA request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "testhostname", "testcluster", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("global service type of ClusterIP not exists", func() {
		It("should failed with A request", func() {
			qname := fmt.Sprintf("%s.%s.svc.%s", "unknownsvc", namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should failed with AAAA request", func() {
			qname := fmt.Sprintf("%s.%s.svc.%s", "unknownsvc", namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

}

func testHeadlessServices() {
	var (
		clusterChaoyang = "chaoyang"
		clusterMinhang  = "minhang"
		svcNginxNorth   = "nginx-north"
		hostname1       = "test01"
		hostname2       = "test02"
		hostname3       = "test03"
		hostname4       = "test04"
		testservice     apis.GlobalService

		testRecorder *dnstest.Recorder
		fabdns       *FabDNS
	)

	BeforeEach(func() {
		testservice = apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcNginxNorth,
				Namespace:   namespaceDefault,
				ClusterName: "haidian",
			},
			Spec: apis.GlobalServiceSpec{
				Type: apis.Headless,
				Ports: []apis.ServicePort{
					{
						Port:     80,
						Name:     "web",
						Protocol: corev1.ProtocolTCP,
					},
				},
				Endpoints: []apis.Endpoint{
					{
						Hostname:  &hostname1,
						Cluster:   "xicheng",
						Region:    "north",
						Zone:      "beijing",
						Addresses: []string{"192.168.1.1"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Hostname:  &hostname2,
						Cluster:   clusterMinhang,
						Region:    "south",
						Zone:      "shanghai",
						Addresses: []string{"192.168.1.2"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Hostname:  &hostname3,
						Cluster:   clusterChaoyang,
						Region:    "north",
						Zone:      "beijing",
						Addresses: []string{"192.168.1.3", "FF01::3"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
					{
						Hostname:  &hostname4,
						Cluster:   clusterChaoyang,
						Region:    "north",
						Zone:      "beijing",
						Addresses: []string{"FF01::4"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Service",
							Name:      svcNginxNorth,
							Namespace: namespaceDefault,
						},
					},
				},
			},
		}
		createGlobalService(testK8sClient, &testservice)

		fabdns = &FabDNS{
			Zones:         []string{testZone},
			TTL:           5,
			Client:        testK8sClient,
			Cluster:       "haidian",
			ClusterZone:   "beijing",
			ClusterRegion: "north",
		}
		testRecorder = dnstest.NewRecorder(&test.ResponseWriter{})
	})

	AfterEach(func() {
		deleteGlobalService(testK8sClient, &testservice)
	})

	When("global service type of Headless exists", func() {
		It("should succeed with A record response", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, clusterMinhang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.A(fmt.Sprintf("%s    5    IN    A    %s", qname, "192.168.1.2")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should succeed with AAAA record response", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname3, clusterChaoyang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeSuccess,
				Answer: []dns.RR{
					test.AAAA(fmt.Sprintf("%s    5    IN    AAAA    %s", qname, "FF01::3")),
				},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("global service type of Headless exists and no A record", func() {
		It("should succeed with A record response", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname4, clusterChaoyang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname:  qname,
				Qtype:  dns.TypeA,
				Rcode:  dns.RcodeSuccess,
				Answer: []dns.RR{},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("global service type of Headless exists and no AAAA record", func() {
		It("should succeed with AAAA record response", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, clusterMinhang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname:  qname,
				Qtype:  dns.TypeAAAA,
				Rcode:  dns.RcodeSuccess,
				Answer: []dns.RR{},
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

	When("global service type of Headless exists and cluster name or hostname of request not exists", func() {
		It("cluster name not exists should failed with A request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, "unknowncluster", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("hostname not exists should failed with A request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "unknownhostname", clusterMinhang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("cluster name not exists should failed with AAAA request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, "unknowncluster", svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("hostname not exists should failed with AAAA request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", "unknownhostname", clusterMinhang, svcNginxNorth, namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

	})

	When("global service type of Headless not exists", func() {
		It("should failed with A request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, clusterMinhang, "unknownsvc", namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})

		It("should failed with AAAA request", func() {
			qname := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname2, clusterMinhang, "unknownsvc", namespaceDefault, testZone)
			testCase := test.Case{
				Qname: qname,
				Qtype: dns.TypeAAAA,
				Rcode: dns.RcodeNameError,
			}
			executeTestCase(fabdns, testRecorder, testCase)
		})
	})

}

func createGlobalService(k8sclient client.Client, globalService *apis.GlobalService) {
	err := k8sclient.Create(context.Background(), globalService, &client.CreateOptions{})
	Expect(err).Should(BeNil())
}

func deleteGlobalService(k8sclient client.Client, globalService *apis.GlobalService) {
	err := k8sclient.Delete(context.Background(), globalService)
	Expect(err).Should(BeNil())
}

func executeTestCase(fabdns *FabDNS, recorder *dnstest.Recorder, testcase test.Case) (rcode int, err error) {
	rcode, err = fabdns.ServeDNS(context.TODO(), recorder, testcase.Msg())
	Expect(rcode).To(Equal(testcase.Rcode))

	if testcase.Rcode == dns.RcodeSuccess {
		Expect(err).To(Succeed())
		Expect(test.SortAndCheck(recorder.Msg, testcase)).To(Succeed())
	} else {
		Expect(err).To(HaveOccurred())
	}
	return rcode, err
}
