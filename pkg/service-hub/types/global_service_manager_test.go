package types_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
	testutil "github.com/FabEdge/fab-dns/pkg/util/test"
)

var _ = Describe("GlobalServiceManager", func() {
	var (
		td                   *testDriver
		serviceFromBeijing   apis.GlobalService
		serviceFromShanghai  apis.GlobalService
		allowCreateNamespace = true
		workNamespace        = "default"
		getNamespace         = testutil.GenerateGetNameFunc("not-exist")
	)

	JustBeforeEach(func() {
		td = newTestDriver(allowCreateNamespace, workNamespace)
		serviceFromBeijing = td.serviceFromBeijing
		serviceFromShanghai = td.serviceFromShanghai
	})

	JustAfterEach(func() {
		td.clearServices()
		allowCreateNamespace = true
		workNamespace = "default"
	})

	Describe("CreateOrMergeGlobalService", func() {
		Context("manager are allowed to create namespace", func() {
			When("namespace exists", func() {
				Context("the corresponding global service does not exist", func() {
					It("will create a corresponding global service under the same namespace", func() {
						td.createOrMergeGlobalService(td.serviceFromBeijing)
						service := td.getService()
						Expect(service.Spec.Type).To(Equal(serviceFromBeijing.Spec.Type))
						Expect(service.Spec.Ports).To(Equal(serviceFromBeijing.Spec.Ports))
						Expect(service.Spec.Endpoints).To(Equal(serviceFromBeijing.Spec.Endpoints))
					})
				})

				Context("the corresponding global service exists", func() {
					JustBeforeEach(func() {
						td.createOrMergeGlobalService(td.serviceFromBeijing)
						td.createOrMergeGlobalService(td.serviceFromShanghai)
					})

					It("will update ports from request", func() {
						service := td.getService()
						Expect(service.Spec.Ports).To(Equal(serviceFromShanghai.Spec.Ports))
						Expect(service.Spec.Ports).NotTo(Equal(serviceFromBeijing.Spec.Ports))
					})

					It("will append endpoints from request", func() {
						service := td.getService()
						Expect(service.Spec.Endpoints).To(ConsistOf(
							serviceFromBeijing.Spec.Endpoints[0],
							serviceFromShanghai.Spec.Endpoints[0],
						))
					})

					When("endpoints of the new service are different from old endpoints", func() {
						It("will remove old endpoints and append new endpoints", func() {
							serviceFromShanghai.Spec.Endpoints = []apis.Endpoint{
								{
									Cluster:   "shanghai",
									Region:    "south",
									Zone:      "shanghai",
									Addresses: []string{"192.168.1.3"},
									TargetRef: &corev1.ObjectReference{
										Kind:      "Service",
										Name:      td.serviceName,
										Namespace: td.namespace,
									},
								},
								{
									Cluster:   "shanghai",
									Region:    "south",
									Zone:      "shanghai",
									Addresses: []string{"192.168.1.4"},
									TargetRef: &corev1.ObjectReference{
										Kind:      "Service",
										Name:      td.serviceName,
										Namespace: td.namespace,
									},
								},
							}
							td.createOrMergeGlobalService(serviceFromShanghai)

							service := td.getService()
							Expect(service.Spec.Ports).To(Equal(serviceFromShanghai.Spec.Ports))
							Expect(service.Spec.Endpoints).To(ConsistOf(
								serviceFromBeijing.Spec.Endpoints[0],
								serviceFromShanghai.Spec.Endpoints[0],
								serviceFromShanghai.Spec.Endpoints[1],
							))
						})
					})
				})
			})

			When("namespace does not exist", func() {
				BeforeEach(func() {
					workNamespace = getNamespace()
				})

				JustBeforeEach(func() {
					td.createOrMergeGlobalService(serviceFromBeijing)
				})

				JustAfterEach(func() {
					td.deleteNamespace(workNamespace)
				})

				It("will create the namespace to which global service belongs", func() {
					testutil.ExpectNamespaceExists(k8sClient, workNamespace)
				})

				It("will create a corresponding global service", func() {
					_ = td.getService()
				})
			})
		})

		Context("manager are not to create namespace", func() {
			BeforeEach(func() {
				allowCreateNamespace = false
			})

			Context("namespace does not exist", func() {
				BeforeEach(func() {
					workNamespace = getNamespace()
				})

				JustBeforeEach(func() {
					Expect(td.manager.CreateOrMergeGlobalService(serviceFromBeijing)).To(HaveOccurred())
				})

				It("global service will not be created", func() {
					td.expectServiceNotFound()
				})

				It("namespace will not be created", func() {
					testutil.ExpectNamespaceNotExists(k8sClient, workNamespace)
				})
			})

			Context("namespace exists", func() {
				It("global service will be created", func() {
					td.createOrMergeGlobalService(td.serviceFromBeijing)
					_ = td.getService()
				})
			})
		})
	})

	Describe("RevokeGlobalService", func() {
		JustBeforeEach(func() {
			td.createOrMergeGlobalService(serviceFromBeijing)
			td.createOrMergeGlobalService(serviceFromShanghai)
			td.revokeGlobalService(serviceFromBeijing)
		})

		It("will remove endpoints of this cluster from specified global service", func() {
			service := td.getService()
			Expect(service.Spec.Ports).To(Equal(serviceFromShanghai.Spec.Ports))
			Expect(service.Spec.Endpoints).To(Equal(serviceFromShanghai.Spec.Endpoints))
		})

		It("the global service will be deleted if no endpoints are left", func() {
			td.revokeGlobalService(serviceFromShanghai)
			td.expectServiceNotFound()
		})

		It("will just return without error if target global service not found", func() {
			td.revokeGlobalService(apis.GlobalService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "not-found",
					Namespace:   "default",
					ClusterName: "test",
				},
			})
		})
	})
})

type testDriver struct {
	serviceFromBeijing  apis.GlobalService
	serviceFromShanghai apis.GlobalService

	manager     types.GlobalServiceManager
	serviceName string
	namespace   string
}

func newTestDriver(allowCreateNamespace bool, namespace string) *testDriver {
	serviceName := "nginx"

	serviceFromBeijing := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceName,
			Namespace:   namespace,
			ClusterName: "beijing",
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
					Cluster:   "beijing",
					Region:    "north",
					Zone:      "beijing",
					Addresses: []string{"192.168.1.1"},
					TargetRef: &corev1.ObjectReference{
						Kind:      "Service",
						Name:      serviceName,
						Namespace: namespace,
					},
				},
			},
		},
	}

	serviceFromShanghai := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceName,
			Namespace:   namespace,
			ClusterName: "shanghai",
		},
		Spec: apis.GlobalServiceSpec{
			Type: apis.ClusterIP,
			Ports: []apis.ServicePort{
				{
					Port:     8080,
					Name:     "web",
					Protocol: corev1.ProtocolTCP,
				},
			},
			Endpoints: []apis.Endpoint{
				{
					Cluster:   "shanghai",
					Region:    "south",
					Zone:      "shanghai",
					Addresses: []string{"192.168.1.2"},
					TargetRef: &corev1.ObjectReference{
						Kind:      "Service",
						Name:      serviceName,
						Namespace: namespace,
					},
				},
			},
		},
	}

	return &testDriver{
		serviceName:         serviceName,
		namespace:           namespace,
		manager:             types.NewGlobalServiceManager(k8sClient, allowCreateNamespace),
		serviceFromBeijing:  serviceFromBeijing,
		serviceFromShanghai: serviceFromShanghai,
	}
}

func (td *testDriver) createOrMergeGlobalService(service apis.GlobalService) {
	Expect(td.manager.CreateOrMergeGlobalService(service)).To(Succeed())
}

func (td *testDriver) revokeGlobalService(svc apis.GlobalService) {
	Expect(td.manager.RevokeGlobalService(svc.ClusterName, svc.Namespace, svc.Name)).To(Succeed())
}

func (td *testDriver) getService() apis.GlobalService {
	return testutil.ExpectGetGlobalService(k8sClient, client.ObjectKey{Name: td.serviceName, Namespace: td.namespace})
}

func (td *testDriver) expectServiceNotFound() {
	testutil.ExpectGlobalServiceNotFound(k8sClient, client.ObjectKey{Name: td.serviceName, Namespace: td.namespace})
}

func (td *testDriver) clearServices() {
	testutil.PurgeAllGlobalServices(k8sClient)
}

func (td *testDriver) createNamespace(name string) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	Expect(k8sClient.Create(context.Background(), &ns)).To(Succeed())
}

func (td *testDriver) deleteNamespace(name string) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	Expect(k8sClient.Delete(context.Background(), &ns)).To(Succeed())
}
