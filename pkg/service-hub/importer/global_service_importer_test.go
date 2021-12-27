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

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/constants"
)

var _ = Describe("GlobalServiceImporter", func() {
	Describe("can import services returned by GetGlobalServices passed in", func() {
		var (
			importer       *globalServiceImporter
			sourceServices *globalServices
		)

		BeforeEach(func() {
			sourceServices = &globalServices{}
			importer = &globalServiceImporter{
				Config: Config{
					Interval:          time.Second,
					GetGlobalServices: sourceServices.GetServices,
				},
				client: k8sClient,
				log:    ctrlpkg.Log,
			}
		})

		When("a local counterpart does not exist", func() {
			var (
				importedService apis.GlobalService
				serviceKey      client.ObjectKey
			)

			BeforeEach(func() {
				importedService = apis.GlobalService{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "nginx",
						Namespace:       "default",
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
				serviceKey = keyFromObject(&importedService)

				sourceServices.AddService(importedService)
				importer.importServices()
			})

			It("will create this global service under the same namespace", func() {
				var localService apis.GlobalService

				Eventually(func() error {
					return k8sClient.Get(context.Background(), serviceKey, &localService)
				}).Should(Succeed())
				Expect(localService.Labels).To(HaveKeyWithValue(constants.KeyOriginResourceVersion, importedService.ResourceVersion))
				Expect(localService.Spec).To(Equal(importedService.Spec))
			})

			When("imported service is different from local counterpart", func() {
				BeforeEach(func() {
					importedService.ResourceVersion = "1234567"
					importedService.Spec.Ports = []apis.ServicePort{
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
					sourceServices.AddService(importedService)
					importer.importServices()
				})

				It("will update the local counterpart", func() {
					var localService apis.GlobalService

					Eventually(func() []apis.ServicePort {
						_ = k8sClient.Get(context.Background(), serviceKey, &localService)
						return localService.Spec.Ports
					}).Should(Equal(importedService.Spec.Ports))
				})
			})

			When("a previous imported global service is gone", func() {
				BeforeEach(func() {
					sourceServices.Remove(importedService)
					importer.importServices()
				})

				It("will delete its local counterpart", func() {
					var localService apis.GlobalService

					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), serviceKey, &localService)
						return errors.IsNotFound(err)
					}).Should(BeTrue(), "should get a not found error")
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
