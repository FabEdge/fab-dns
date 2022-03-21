package fabdns

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	Context("Parsing valid requests", testValidRequests)
	Context("Parsing invalid requests", testInvalidRequests)
})

func testValidRequests() {

	type testExpected struct {
		qname    string
		expected string
	}

	When("ClusterIP svc request", func() {
		It("should be no error", func() {
			test := testExpected{"myservice.mynamespace.svc." + testZone, "..myservice.mynamespace.svc"}
			req, err := parseRequest(test.qname, testZone)
			Expect(err).To(BeNil())
			Expect(req.String()).To(Equal(test.expected))
		})
	})

	When("Headless svc request", func() {
		It("should be no error", func() {
			test := testExpected{"hostname.mycluster.myservice.mynamespace.svc." + testZone, "hostname.mycluster.myservice.mynamespace.svc"}
			req, err := parseRequest(test.qname, testZone)
			Expect(err).To(BeNil())
			Expect(req.String()).To(Equal(test.expected))
		})
	})
}

func testInvalidRequests() {

	When("SVC request too lang", func() {
		It("should be error", func() {
			qname := "too.lang.request.myservice.mynamespace.svc." + testZone
			_, err := parseRequest(qname, testZone)
			Expect(err).Should(HaveOccurred())
		})
	})

	When("request not for svc", func() {
		It("should be error", func() {
			qname := "myservice.mynamespace.pod." + testZone
			_, err := parseRequest(qname, testZone)
			Expect(err).Should(HaveOccurred())
		})
	})
}
