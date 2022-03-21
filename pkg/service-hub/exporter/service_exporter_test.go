package exporter

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	testutil "github.com/fabedge/fab-dns/pkg/util/test"
)

var _ = Describe("ServiceExporter", func() {
	var td *testDriver

	BeforeEach(func() {
		td = newTestDriver()
		td.start()
	})

	AfterEach(func() {
		td.teardown()
	})

	Context("A service with a ClusterIP", func() {
		var svc corev1.Service

		BeforeEach(func() {
			svc = corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "default",
					Labels: map[string]string{
						labelGlobalService: "true",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "default",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     8080,
							Name:     "health",
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
		})

		When("it is marked as global-service", func() {
			BeforeEach(func() {
				td.createObject(&svc)
				td.expectExporterReconcile(&svc)
			})

			It("will export this service as global service", func() {
				td.expectServiceExported(&svc, nil)
			})

			When("this service's global-service marker is removed", func() {
				Specify("it and endpoints will be revoked", func() {
					svc.Labels = nil
					td.updateObject(&svc)
					td.expectExporterReconcile(&svc)
					td.expectServiceNotExported(&svc)
				})
			})
		})

		When("it is not marked as global service", func() {
			It("will be ignored and will not be exported", func() {
				svc.Labels = nil
				td.createObject(&svc)
				td.expectExporterReconcile(&svc)

				td.expectServiceNotExported(&svc)
			})
		})
	})

	Context("A headless service", func() {
		var svc corev1.Service
		var endpointslice discoveryv1.EndpointSlice

		BeforeEach(func() {
			hostname1 := "mysql-1"
			hostname2 := "mysql-2"
			managedByController := true

			port := corev1.ServicePort{
				Name:     "default",
				Port:     3306,
				Protocol: corev1.ProtocolTCP,
			}
			svc = corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mysql",
					Namespace: "default",
					Labels: map[string]string{
						labelGlobalService: "true",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: corev1.ClusterIPNone,
					Ports:     []corev1.ServicePort{port},
				},
			}
			endpointslice = discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mysql-123456",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "mysql",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Service",
							Name:       svc.Name,
							UID:        "123456",
							Controller: &managedByController,
						},
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Ports: []discoveryv1.EndpointPort{
					{
						Name:     &port.Name,
						Port:     &port.Port,
						Protocol: &port.Protocol,
					},
				},
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"192.168.1.1"},
						Hostname:  &hostname1,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      hostname1,
							Namespace: "default",
						},
					},
					{
						Addresses: []string{"192.168.1.2"},
						Hostname:  &hostname2,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      hostname2,
							Namespace: "default",
						},
					},
				},
			}
		})

		When("it is marked as global-service", func() {
			BeforeEach(func() {
				td.createObject(&svc)
				td.expectExporterReconcile(&svc)

				td.createObject(&endpointslice)
				td.expectExporterReconcile(&svc)
			})

			It("will export this service as global services", func() {
				td.expectServiceExported(&svc, &endpointslice)
			})

			When("the global-service marker is removed", func() {
				Specify("it and endpoints will be revoked", func() {
					svc.Labels = nil
					td.updateObject(&svc)
					td.expectExporterReconcile(&svc)

					td.expectServiceNotExported(&svc)
				})
			})
		})

		When("it is not marked as global service", func() {
			It("will be ignored and will not be exported", func() {
				svc.Labels = nil
				td.createObject(&svc)
				td.expectExporterReconcile(&svc)

				td.expectServiceNotExported(&svc)
			})
		})
	})
})

type testDriver struct {
	cluster string
	zone    string
	region  string

	exporter            *serviceExporter
	exporterRequestChan chan reconcile.Request
	lostServiceRevoker  *lostServiceRevoker
	checkerRequestChan  chan reconcile.Request
	manager             manager.Manager

	exportedGlobalService *apis.GlobalService
	stopManager           func()
}

func newTestDriver() *testDriver {
	mgr, err := manager.New(kubeConfig, manager.Options{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	})
	Expect(err).To(BeNil())

	cluster, zone, region := "fabedge", "haidian", "region"
	cfg := Config{
		ClusterName: cluster,
		Zone:        zone,
		Region:      region,
		Manager:     mgr,
	}
	exporter := newServiceExporter(cfg)
	reconciler, exporterReqChan := testutil.WrapReconcile(exporter)
	Expect(addExporterToManager(mgr, reconciler)).To(Succeed())

	revoker := newLostServiceRevoker(cfg)
	reconciler, checkerReqChan := testutil.WrapReconcile(revoker)
	Expect(addDiffCheckerToManager(mgr, reconciler)).To(Succeed())

	td := &testDriver{
		cluster:             cluster,
		zone:                zone,
		region:              region,
		manager:             mgr,
		exporter:            exporter,
		exporterRequestChan: exporterReqChan,
		lostServiceRevoker:  revoker,
		checkerRequestChan:  checkerReqChan,
	}
	exporter.ExportGlobalService = td.exportGlobalService
	exporter.RevokeGlobalService = td.revokeGlobalService
	revoker.ExportGlobalService = td.exportGlobalService
	revoker.RevokeGlobalService = td.revokeGlobalService

	return td
}

func (td *testDriver) start() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(td.manager.Start(ctx))
	}()
	td.stopManager = cancel
}

func (td *testDriver) createObject(obj client.Object) {
	Expect(k8sClient.Create(context.TODO(), obj)).To(Succeed())
}

func (td *testDriver) updateObject(obj client.Object) {
	Expect(k8sClient.Update(context.TODO(), obj)).To(Succeed())
}

func (td *testDriver) DeleteObject(obj client.Object) {
	Expect(k8sClient.Delete(context.TODO(), obj)).To(Succeed())
}

func (td *testDriver) GetService(key client.ObjectKey) (svc corev1.Service) {
	Expect(k8sClient.Get(context.TODO(), key, &svc)).To(Succeed())
	return svc
}

func (td *testDriver) exportGlobalService(svc apis.GlobalService) error {
	td.exportedGlobalService = &svc
	return nil
}

func (td *testDriver) revokeGlobalService(clusterName, namespace, name string) error {
	Expect(clusterName).To(Equal(td.cluster))
	gs := td.exportedGlobalService
	if gs == nil {
		return nil
	}

	if gs.Namespace == namespace && gs.Name == name {
		td.exportedGlobalService = nil
	}

	return nil
}

func (td *testDriver) teardown() {
	td.stopManager()
	testutil.PurgeAllGlobalServices(k8sClient)
	testutil.PurgeAllServices(k8sClient)
	testutil.PurgeAllEndpointSlices(k8sClient)
}

func (td *testDriver) expectExporterReconcile(obj client.Object) {
	Eventually(td.exporterRequestChan, time.Second).Should(Receive(Equal(reconcile.Request{
		NamespacedName: client.ObjectKey{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
	})))
}

func (td *testDriver) expectRevokerReconcile(obj client.Object) {
	Eventually(td.checkerRequestChan, time.Second).Should(Receive(Equal(reconcile.Request{
		NamespacedName: client.ObjectKey{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
	})))
}

func (td *testDriver) expectServiceExported(svc *corev1.Service, endpointslice *discoveryv1.EndpointSlice) {
	exportedService := td.exportedGlobalService
	Expect(exportedService.ClusterName).To(Equal(td.cluster))
	Expect(len(exportedService.Spec.Ports)).To(Equal(len(svc.Spec.Ports)))
	Expect(td.exporter.serviceKeySet.Has(client.ObjectKey{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	})).To(BeTrue())

	for i := range exportedService.Spec.Ports {
		p1 := exportedService.Spec.Ports[i]
		p2 := &svc.Spec.Ports[i]

		Expect(p1.Port).To(Equal(p2.Port))
		Expect(p1.Name).To(Equal(p2.Name))
		Expect(p1.Protocol).To(Equal(p2.Protocol))
		Expect(p1.AppProtocol).To(Equal(p2.AppProtocol))
	}

	if endpointslice == nil {
		Expect(exportedService.Spec.Type).To(Equal(apis.ClusterIP))
		Expect(len(exportedService.Spec.Endpoints)).To(Equal(1))
		Expect(exportedService.Spec.Endpoints[0]).To(Equal(apis.Endpoint{
			Cluster:   td.cluster,
			Zone:      td.zone,
			Region:    td.region,
			Addresses: []string{svc.Spec.ClusterIP},
		}))
	} else {
		Expect(exportedService.Spec.Type).To(Equal(apis.Headless))
		Expect(len(exportedService.Spec.Endpoints)).To(Equal(len(endpointslice.Endpoints)))
		for i := range exportedService.Spec.Endpoints {
			ep1 := exportedService.Spec.Endpoints[i]
			ep2 := endpointslice.Endpoints[i]

			Expect(ep1.Cluster).To(Equal(td.cluster))
			Expect(ep1.Zone).To(Equal(td.zone))
			Expect(ep1.Region).To(Equal(td.region))
			Expect(ep1.Addresses).To(Equal(ep2.Addresses))
			Expect(ep1.Hostname).To(Equal(ep2.Hostname))
			Expect(ep1.TargetRef).To(Equal(ep2.TargetRef))
		}
	}
}

func (td *testDriver) expectServiceNotExported(svc *corev1.Service) {
	Expect(td.exportedGlobalService).To(BeNil())
	Expect(td.exporter.serviceKeySet.Has(client.ObjectKey{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	})).To(BeFalse())
}
