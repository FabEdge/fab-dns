package fabdns

import (
	"context"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
