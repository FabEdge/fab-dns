package exporter

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

var _ = Describe("LostServiceRevoker", func() {
	Describe("watch global service events for each global service", func() {
		var (
			td            *testDriver
			service       corev1.Service
			globalService apis.GlobalService
		)

		BeforeEach(func() {
			td = newTestDriver()
			td.start()
			service = corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP:  "10.233.46.189",
					ClusterIPs: []string{"10.233.46.189", "fd85:ee78:d8a6:8607::14c0"},
					Ports: []corev1.ServicePort{
						{
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
			globalService = apis.GlobalService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      service.Name,
					Namespace: service.Namespace,
				},
				Spec: apis.GlobalServiceSpec{
					Type: apis.ClusterIP,
					Ports: []apis.ServicePort{
						{
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Endpoints: nil,
				},
			}
		})

		AfterEach(func() {
			td.teardown()
		})

		JustBeforeEach(func() {
			// we need a fake exported global service for later expects
			_ = td.exportGlobalService(context.Background(), globalService)

			td.createObject(&globalService)
			td.expectRevokerReconcile(&globalService)
		})

		JustAfterEach(func() {
			td.teardown()
		})

		Context("this global service has endpoints in current cluster", func() {
			BeforeEach(func() {
				globalService.Spec.Endpoints = append(globalService.Spec.Endpoints,
					apis.Endpoint{
						Cluster:   td.cluster,
						Zone:      td.zone,
						Region:    td.region,
						Addresses: service.Spec.ClusterIPs,
					})
			})

			When("the service referenced by the endpoint is no longer global service", func() {
				BeforeEach(func() {
					td.createObject(&service)
					td.expectExporterReconcile(&service)
				})

				It("will revoke this service", func() {
					td.expectServiceNotExported(&service)
				})
			})

			When("the service referenced by the endpoint is not found", func() {
				It("will revoke this service", func() {
					td.expectServiceNotExported(&service)
				})
			})
		})

		Context("a global service has no endpoint in current cluster", func() {
			BeforeEach(func() {
				globalService.Spec.Endpoints = append(globalService.Spec.Endpoints, apis.Endpoint{
					Cluster:   "some-cluster",
					Zone:      "default",
					Region:    "default",
					Addresses: []string{"192.168.1.1"},
				})
			})

			It("nothing will happened", func() {
				Expect(td.exportedGlobalService).NotTo(BeNil())
			})
		})
	})

})
