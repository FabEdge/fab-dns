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
		qname string
		rr    recordRequest
	}

	When("ClusterIP svc request", func() {
		It("should be no error", func() {
			test := testExpected{"myservice.mynamespace.svc." + testZone,
				recordRequest{
					service:   "myservice",
					namespace: "mynamespace",
					cluster:   "",
					isAdHoc:   false,
					hostname:  "",
				},
			}
			req, err := parseRequest(test.qname)
			Expect(err).To(BeNil())
			Expect(req).To(Equal(test.rr))
		})
	})

	When("Headless svc request", func() {
		It("should be no error", func() {
			test := testExpected{"hostname.mycluster.myservice.mynamespace.svc." + testZone,
				recordRequest{
					service:   "myservice",
					namespace: "mynamespace",
					cluster:   "mycluster",
					isAdHoc:   false,
					hostname:  "hostname",
				},
			}

			req, err := parseRequest(test.qname)
			Expect(err).To(BeNil())
			Expect(req).To(Equal(test.rr))
		})
	})

	When("ad-hoc clusterIP svc request", func() {
		It("should no error", func() {
			test := testExpected{"myservice.mynamespace.mycluster." + testZone,
				recordRequest{
					service:   "myservice",
					namespace: "mynamespace",
					cluster:   "mycluster",
					isAdHoc:   true,
					hostname:  "",
				},
			}

			req, err := parseRequest(test.qname)
			Expect(err).To(BeNil())
			Expect(req).To(Equal(test.rr))
		})
	})
}

func testInvalidRequests() {

	When("request too long", func() {
		It("should be error", func() {
			qname := "too.lang.request.myservice.mynamespace.svc." + testZone
			_, err := parseRequest(qname)
			Expect(err).Should(HaveOccurred())
		})
	})

	When("request too short", func() {
		It("should be error", func() {
			qname := "mynamespace.svc." + testZone
			_, err := parseRequest(qname)
			Expect(err).Should(HaveOccurred())
		})
	})

	When("request too short", func() {
		It("should be error", func() {
			qname := "svc." + testZone
			_, err := parseRequest(qname)
			Expect(err).Should(HaveOccurred())
		})
	})
}
