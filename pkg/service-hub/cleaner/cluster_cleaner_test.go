package cleaner

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

var _ = Describe("ClusterCleaner", func() {
	var (
		cleaner        *clusterCleaner
		store          *types.ClusterStore
		serviceManager types.GlobalServiceManager
	)

	BeforeEach(func() {
		store = types.NewClusterStore()
		serviceManager = types.NewGlobalServiceManager(k8sClient, true)
		cleaner = &clusterCleaner{
			client: k8sClient,
			Config: Config{
				Interval:            time.Second,
				RequestTimeout:      time.Second,
				Store:               store,
				RevokeGlobalService: serviceManager.RevokeGlobalService,
			},
			log: ctrlpkg.Log,
		}
	})

	Describe("cleanExpiredClusterEndpoints", func() {
		var (
			globalService apis.GlobalService
			serviceKey    client.ObjectKey
			cluster       *types.Cluster
		)

		BeforeEach(func() {
			cluster = store.New("fabedge")

			globalService = apis.GlobalService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "default",
				},
				Spec: apis.GlobalServiceSpec{
					Type: apis.ClusterIP,
					Ports: []apis.ServicePort{
						{
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Endpoints: []apis.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Cluster:   cluster.Name(),
							Zone:      "haidian",
							Region:    "beijing",
						},
						{
							Addresses: []string{"192.168.1.2"},
							Cluster:   "not-expired",
							Zone:      "chaoyang",
							Region:    "beijing",
						},
					},
				},
			}
			serviceKey = client.ObjectKey{
				Name:      globalService.Name,
				Namespace: globalService.Namespace,
			}

			Expect(serviceManager.CreateOrMergeGlobalService(context.Background(), globalService)).To(Succeed())
			cluster.AddServiceKey(serviceKey)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), &globalService)).To(Succeed())
		})

		It("will revoke global services of expired cluster", func() {
			cluster.SetExpireTime(time.Now().Add(-time.Second))
			cleaner.cleanExpiredClusterEndpoints()

			var currentGlobalService apis.GlobalService
			Expect(k8sClient.Get(context.Background(), serviceKey, &currentGlobalService)).To(Succeed())

			Expect(currentGlobalService.Spec.Type).To(Equal(globalService.Spec.Type))
			Expect(currentGlobalService.Spec.Ports).To(Equal(globalService.Spec.Ports))
			Expect(currentGlobalService.Spec.Endpoints).NotTo(Equal(globalService.Spec.Endpoints))
			Expect(currentGlobalService.Spec.Endpoints[0]).To(Equal(globalService.Spec.Endpoints[1]))
		})
	})
})
