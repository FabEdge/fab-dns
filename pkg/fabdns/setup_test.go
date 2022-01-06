package fabdns

import (
	"context"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

type fakeHandler struct{}

func (f *fakeHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return dns.RcodeSuccess, nil
}

func (f *fakeHandler) Name() string { return "fakehandler" }

var _ = Describe("Setup", func() {
	Context("Parsing correct configurations", testCorrectConfig)
	Context("Parsing incorrect configurations", testIncorrectConfig)
	Context("Plugin registration", testPluginRegistration)
})

func testCorrectConfig() {
	var (
		fabdns *FabDNS
		config string
	)
	JustBeforeEach(func() {
		var err error
		fabdns, err = fabdnsParse(caddy.NewTestController("dns", config))
		Expect(err).To(Succeed())
	})

	When("fabdns without arguments", func() {
		BeforeEach(func() {
			config = `fabdns`
		})
		It("should succeed with empty zones", func() {
			Expect(fabdns.Zones).To(BeEmpty())
		})
	})

	When("fabdns zone and fallthrough zone arguments are not specified", func() {
		BeforeEach(func() {
			config = `fabdns {
				fallthrough
			}`
		})
		It("should succeed with the root fallthrough zones", func() {
			Expect(fabdns.Zones).To(BeEmpty())
			Expect(fabdns.Fall).To(Equal(fall.Root))
		})
	})

	When("fabdns zone is specified", func() {
		BeforeEach(func() {
			config = `fabdns global`
		})
		It("should succeed with the specified zones", func() {
			Expect(fabdns.Zones).To(Equal([]string{"global."}))
		})
	})

	When("fabdns zone and fallthrough zone arguments are specified", func() {
		BeforeEach(func() {
			config = `fabdns global test.org {
				fallthrough test.org
			}`
		})
		It("should succeed with the specified zones", func() {
			Expect(fabdns.Zones).To(Equal([]string{"global.", "test.org."}))
			Expect(fabdns.Fall.Zones).To(Equal([]string{"test.org."}))
		})
	})

	When("fabdns kubeconfig and masterurl are specified", func() {
		var (
			oldBuildConfig func(masterUrl string, kubeconfigPath string) (*rest.Config, error)
			kubeconfig     string
			masterurl      string
		)
		BeforeEach(func() {
			config = `fabdns {
				kubeconfig /tmp/kubeconfigPath
				masterurl https://1.1.1.1:6443
			}`
			oldBuildConfig = buildConfigFromFlags
			buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
				masterurl = masterUrl
				kubeconfig = kubeconfigPath
				return testCfg, nil
			}
		})
		AfterEach(func() {
			buildConfigFromFlags = oldBuildConfig
		})
		It("should succeed with the specified kubeconfig and masterurl", func() {
			Expect(kubeconfig).To(Equal("/tmp/kubeconfigPath"))
			Expect(masterurl).To(Equal("https://1.1.1.1:6443"))
		})
	})

	When("fabdns cluster location infos are specified", func() {
		BeforeEach(func() {
			config = `fabdns {
				cluster haidian
				cluster-zone beijing
				cluster-region north
			}`
		})
		It("should succeed with the specified cluster location infos", func() {
			Expect(fabdns.Cluster).To(Equal("haidian"))
			Expect(fabdns.ClusterZone).To(Equal("beijing"))
			Expect(fabdns.ClusterRegion).To(Equal("north"))
		})
	})

	When("fabdns ttl is specified", func() {
		BeforeEach(func() {
			config = `fabdns {
				ttl 30
			}`
		})
		It("should succeed with the specified cluster location infos", func() {
			Expect(fabdns.TTL).To(Equal(uint32(30)))
		})
	})
}

func testIncorrectConfig() {
	var (
		parseErr error
		config   string
	)
	JustBeforeEach(func() {
		_, parseErr = fabdnsParse(caddy.NewTestController("dns", config))
	})

	When("an unexpected argument is specified", func() {
		BeforeEach(func() {
			config = `fabdns {
                notexist
		    } notexist`
		})

		It("should return an appropriate plugin error", func() {
			Expect(parseErr.Error()).To(ContainSubstring("notexist"))
		})
	})

	When("fabdns kubeconfig specified unexpected args", func() {
		BeforeEach(func() {
			config = `fabdns {
				kubeconfig /tmp/kubeconfigPath unexpectedArg
			}`
		})
		It("should return arguments error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("unexpected ttl is specified", func() {
		BeforeEach(func() {
			config = `fabdns {
				ttl 5000
			}`
		})
		It("should return ttl error", func() {
			Expect(parseErr.Error()).To(ContainSubstring("must be in range"))
		})
	})
}

func testPluginRegistration() {
	It("register fabdns plugin with DNS server should succeed", func() {
		controller := caddy.NewTestController("dns", PluginName)
		err := setup(controller)
		Expect(err).To(Succeed())

		plugins := dnsserver.GetConfig(controller).Plugin
		Expect(plugins).To(HaveLen(1))

		fake := &fakeHandler{}
		handler := plugins[0](fake)
		fabdns, ok := handler.(*FabDNS)
		Expect(ok).To(BeTrue())
		Expect(fabdns.Next).To(BeIdenticalTo(fake))
	})
}
