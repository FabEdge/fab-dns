package importer

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/constants"
	testutil "github.com/fabedge/fab-dns/pkg/util/test"
)

var _ = Describe("GlobalServiceImporter", func() {
	var (
		importer             *globalServiceImporter
		sourceServices       *globalServices
		allowCreateNameSpace = true
		workNamespace        = "default"
		getNamespace         = testutil.GenerateGetNameFunc("not-exist")
	)

	JustBeforeEach(func() {
		sourceServices = &globalServices{}
		importer = &globalServiceImporter{
			Config: Config{
				Interval:             time.Second,
				GetGlobalServices:    sourceServices.GetServices,
				AllowCreateNamespace: allowCreateNameSpace,
			},
			client: k8sClient,
			log:    ctrlpkg.Log,
		}
	})

	JustAfterEach(func() {
		testutil.PurgeAllGlobalServices(k8sClient)
	})

	Describe("importServices", func() {
		When("a local counterpart does not exist", func() {
			var (
				serviceToImport, serviceToDelete       apis.GlobalService
				serviceKeyToImport, serviceKeyToDelete client.ObjectKey
			)

			JustBeforeEach(func() {
				serviceToImport, serviceKeyToImport = makeupGlobalServiceNginx(workNamespace)
				sourceServices.AddService(serviceToImport)

				serviceToDelete, serviceKeyToDelete = makeupGlobalService("to-delete", workNamespace)
				importer.createOrUpdateGlobalService(serviceToDelete)

				importer.importServices()
			})

			It("will save global services", func() {
				var localService apis.GlobalService
				Eventually(func() error {
					return k8sClient.Get(context.Background(), serviceKeyToImport, &localService)
				}).Should(Succeed())
				expectGlobalServiceSaved(serviceToImport)
			})

			It("will delete global services not imported anymore", func() {
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), serviceKeyToDelete, &apis.GlobalService{})
					return errors.IsNotFound(err)
				}).Should(BeTrue(), "should get a not found error")
			})
		})
	})

	Describe("createOrUpdateGlobalService", func() {
		var (
			globalService apis.GlobalService
			serviceKey    client.ObjectKey
		)

		BeforeEach(func() {
			workNamespace = getNamespace()
		})

		AfterEach(func() {
			workNamespace = "default"
		})

		Context("when are allowed to create namespace", func() {
			JustBeforeEach(func() {
				globalService, serviceKey = makeupGlobalServiceNginx(workNamespace)
				importer.createOrUpdateGlobalService(globalService)
			})

			It("will create namespace if it does not exist", func() {
				testutil.ExpectNamespaceExists(k8sClient, workNamespace)
			})

			It("create global service", func() {
				expectGlobalServiceSaved(globalService)
			})

			It("can update the global service", func() {
				changeServicePorts(&globalService)
				importer.createOrUpdateGlobalService(globalService)
				expectGlobalServiceSaved(globalService)
			})
		})

		Context("when are not allowed to create namespace", func() {
			BeforeEach(func() {
				allowCreateNameSpace = false
			})

			AfterEach(func() {
				allowCreateNameSpace = true
			})

			It("won't create namespace", func() {
				globalService, serviceKey = makeupGlobalServiceNginx(getNamespace())
				importer.createOrUpdateGlobalService(globalService)

				testutil.ExpectNamespaceNotExists(k8sClient, workNamespace)
			})

			When("namespace exists", func() {
				It("save global service", func() {
					globalService, serviceKey = makeupGlobalServiceNginx("default")
					importer.createOrUpdateGlobalService(globalService)
					expectGlobalServiceSaved(globalService)
				})
			})

			When("namespace does not exist", func() {
				It("won't save global service", func() {
					globalService, serviceKey = makeupGlobalServiceNginx(getNamespace())
					importer.createOrUpdateGlobalService(globalService)

					testutil.ExpectGlobalServiceNotFound(k8sClient, serviceKey)
				})
			})
		})
	})
})

type globalServices struct {
	services map[client.ObjectKey]apis.GlobalService
	lock     sync.Mutex
}

func (gss *globalServices) AddService(service apis.GlobalService) {
	gss.lock.Lock()
	defer gss.lock.Unlock()

	if gss.services == nil {
		gss.services = make(map[client.ObjectKey]apis.GlobalService)
	}
	key := client.ObjectKey{
		Name:      service.Name,
		Namespace: service.Namespace,
	}
	gss.services[key] = service
}

func (gss *globalServices) Remove(service apis.GlobalService) {
	gss.lock.Lock()
	defer gss.lock.Unlock()

	if gss.services == nil {
		return
	}

	key := client.ObjectKey{
		Name:      service.Name,
		Namespace: service.Namespace,
	}
	delete(gss.services, key)
}

func (gss *globalServices) GetServices() ([]apis.GlobalService, error) {
	gss.lock.Lock()
	defer gss.lock.Unlock()

	var services []apis.GlobalService
	for _, svc := range gss.services {
		services = append(services, svc)
	}

	return services, nil
}

func makeupGlobalServiceNginx(namespace string) (apis.GlobalService, client.ObjectKey) {
	return makeupGlobalService("nginx", namespace)
}

func makeupGlobalService(name, namespace string) (apis.GlobalService, client.ObjectKey) {
	gs := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "123456",
		},
		Spec: apis.GlobalServiceSpec{
			Type: apis.ClusterIP,
			Ports: []apis.ServicePort{
				{
					Name:     "web",
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
			},
			Endpoints: []apis.Endpoint{
				{
					Cluster:   "fabedge",
					Zone:      "haidian",
					Region:    "beijing",
					Addresses: []string{"192.168.1.1"},
				},
			},
		},
	}

	return gs, keyFromObject(&gs)
}

func changeServicePorts(svc *apis.GlobalService) {
	svc.ResourceVersion = "1234567"
	svc.Spec.Ports = []apis.ServicePort{
		{
			Name:     "web",
			Port:     80,
			Protocol: corev1.ProtocolTCP,
		},
		{
			Name:     "health",
			Port:     8080,
			Protocol: corev1.ProtocolTCP,
		},
	}
}

func expectGlobalServiceSaved(expectedService apis.GlobalService) {
	key := client.ObjectKey{
		Name:      expectedService.Name,
		Namespace: expectedService.Namespace,
	}
	savedService := testutil.ExpectGetGlobalService(k8sClient, key)
	Expect(savedService.Labels).To(HaveKeyWithValue(constants.KeyOriginResourceVersion, expectedService.ResourceVersion))
	Expect(savedService.Spec).To(Equal(expectedService.Spec))
}
