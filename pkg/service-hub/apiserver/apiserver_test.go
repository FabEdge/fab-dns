package apiserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/service-hub/apiserver"
	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
)

const (
	serviceNginx     = "nginx"
	namespaceDefault = "default"
	namespaceTest    = "test"
)

var _ = Describe("APIServer", func() {

	var (
		td                  *testDriver
		serviceFromBeijing  apis.GlobalService
		serviceFromShanghai apis.GlobalService
	)

	BeforeEach(func() {
		td = newTestDriver()
		serviceFromBeijing = td.serviceFromBeijing
		serviceFromShanghai = td.serviceFromShanghai
	})

	AfterEach(func() {
		td.clearServices()
	})

	When("receive a heartbeat request", func() {

		It("will change expire time of cluster in cluster store", func() {
			clusterName := "test"
			resp := td.heartbeat(clusterName)
			Expect(resp.Code).To(Equal(http.StatusNoContent))

			cluster := td.clusterStore.Get(clusterName)
			Expect(cluster.ExpireTime().IsZero()).To(BeFalse())
		})
	})

	When("receive a upload request for a exported service from a cluster", func() {
		Context("the corresponding global service does not exist", func() {
			BeforeEach(func() {
				resp := td.uploadGlobalService(td.serviceFromBeijing)
				Expect(resp.Code).To(Equal(http.StatusNoContent))
			})

			It("will create a corresponding global service under the same namespaceDefault", func() {
				service := td.getService()
				Expect(service.Spec.Type).To(Equal(serviceFromBeijing.Spec.Type))
				Expect(service.Spec.Ports).To(Equal(serviceFromBeijing.Spec.Ports))
				Expect(service.Spec.Endpoints).To(Equal(serviceFromBeijing.Spec.Endpoints))
			})

			It("will add service's key to cluster in cluster store", func() {
				svc := td.serviceFromBeijing
				cluster := td.clusterStore.Get(svc.ClusterName)
				Expect(cluster).NotTo(BeNil())

				keys := cluster.GetAllServiceKeys()
				Expect(keys).To(ConsistOf(client.ObjectKey{
					Name:      svc.Name,
					Namespace: svc.Namespace,
				}))
			})
		})

		Context("the corresponding global service exists", func() {
			BeforeEach(func() {
				resp := td.uploadGlobalService(td.serviceFromBeijing)
				Expect(resp.Code).To(Equal(http.StatusNoContent))

				resp = td.uploadGlobalService(td.serviceFromShanghai)
				Expect(resp.Code).To(Equal(http.StatusNoContent))
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

			When("endpoints of the uploaded service are different from old endpoints", func() {
				It("will remove old endpoints and append new endpoints", func() {
					serviceFromShanghai.Spec.Endpoints = []apis.Endpoint{
						{
							Cluster:   "shanghai",
							Region:    "south",
							Zone:      "shanghai",
							Addresses: []string{"192.168.1.3"},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Service",
								Name:      serviceNginx,
								Namespace: namespaceDefault,
							},
						},
						{
							Cluster:   "shanghai",
							Region:    "south",
							Zone:      "shanghai",
							Addresses: []string{"192.168.1.4"},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Service",
								Name:      serviceNginx,
								Namespace: namespaceDefault,
							},
						},
					}
					resp := td.uploadGlobalService(serviceFromShanghai)
					Expect(resp.Code).To(Equal(http.StatusNoContent))

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

	When("receive a delete endpoints request from a cluster", func() {
		BeforeEach(func() {
			resp := td.uploadGlobalService(serviceFromBeijing)
			Expect(resp.Code).To(Equal(http.StatusNoContent))

			resp = td.uploadGlobalService(serviceFromShanghai)
			Expect(resp.Code).To(Equal(http.StatusNoContent))

			resp = td.removeEndpoints(serviceFromBeijing.ClusterName)
			Expect(resp.Code).To(Equal(http.StatusNoContent))
		})

		It("will remove endpoints of this cluster from specified global service", func() {
			service := td.getService()
			Expect(service.Spec.Ports).To(Equal(serviceFromShanghai.Spec.Ports))
			Expect(service.Spec.Endpoints).To(Equal(serviceFromShanghai.Spec.Endpoints))
		})

		It("will remove service key from cluster in cluster store", func() {
			clusterName := serviceFromBeijing.ClusterName
			cluster := td.clusterStore.Get(clusterName)
			keys := cluster.GetAllServiceKeys()
			Expect(keys).To(BeNil())
		})

		It("the global service will be deleted if no endpoints are left", func() {
			resp := td.removeEndpoints(serviceFromShanghai.ClusterName)
			Expect(resp.Code).To(Equal(http.StatusNoContent))

			td.ExpectServiceNotFound()
		})
	})

	When("receive a get all endpoints", func() {
		BeforeEach(func() {
			td.createNamespace(namespaceTest)
			serviceFromShanghai.Namespace = namespaceTest

			td.uploadGlobalService(serviceFromBeijing)
			td.uploadGlobalService(serviceFromShanghai)
		})

		AfterEach(func() {
			td.deleteNamespace(namespaceTest)
			td.clearServices()
		})

		It("will return all global services under every namespace", func() {
			services := td.downloadAllGlobalServices("beijing")

			for _, svc := range services {
				if svc.Namespace == namespaceDefault {
					Expect(svc.Name).To(Equal(serviceNginx))
					Expect(svc.Spec.Ports).To(Equal(serviceFromBeijing.Spec.Ports))
					Expect(svc.Spec.Endpoints).To(Equal(serviceFromBeijing.Spec.Endpoints))
				}

				if svc.Namespace == namespaceTest {
					Expect(svc.Name).To(Equal(serviceNginx))
					Expect(svc.Spec.Ports).To(Equal(serviceFromShanghai.Spec.Ports))
					Expect(svc.Spec.Endpoints).To(Equal(serviceFromShanghai.Spec.Endpoints))
				}
			}
		})
	})
})

type testDriver struct {
	server              *http.Server
	serviceFromBeijing  apis.GlobalService
	serviceFromShanghai apis.GlobalService
	clusterStore        *types.ClusterStore

	serviceName string
	namespace   string
}

func newTestDriver() *testDriver {
	clusterStore := types.NewClusterStore()
	server, err := apiserver.New(apiserver.Config{
		Address:               "localhost:3000",
		Log:                   klogr.New(),
		Client:                k8sClient,
		ClusterStore:          clusterStore,
		ClusterExpireDuration: 5 * time.Second,
	})
	Expect(err).Should(Succeed())

	serviceFromBeijing := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceNginx,
			Namespace:   namespaceDefault,
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
						Name:      serviceNginx,
						Namespace: namespaceDefault,
					},
				},
			},
		},
	}

	serviceFromShanghai := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceNginx,
			Namespace:   namespaceDefault,
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
						Name:      serviceNginx,
						Namespace: namespaceDefault,
					},
				},
			},
		},
	}

	return &testDriver{
		server:              server,
		serviceName:         serviceNginx,
		namespace:           namespaceDefault,
		serviceFromBeijing:  serviceFromBeijing,
		serviceFromShanghai: serviceFromShanghai,
		clusterStore:        clusterStore,
	}
}

func (td *testDriver) heartbeat(clusterName string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodGet, apiserver.PathHeartbeat, nil)
	req.Header.Add(apiserver.HeaderClusterName, clusterName)

	return td.sendRequest(req)
}

func (td *testDriver) uploadGlobalService(svc apis.GlobalService) *httptest.ResponseRecorder {
	endpointsJson, _ := json.Marshal(svc)

	reqBody := bytes.NewBuffer(endpointsJson)
	req, _ := http.NewRequest(http.MethodPost, apiserver.PathGlobalServices, reqBody)
	req.Header.Add(apiserver.HeaderClusterName, svc.ClusterName)

	return td.sendRequest(req)
}

func (td *testDriver) removeEndpoints(cluster string) *httptest.ResponseRecorder {
	url := fmt.Sprintf("%s/%s/%s", apiserver.PathGlobalServices, td.namespace, td.serviceName)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	req.Header.Add(apiserver.HeaderClusterName, cluster)

	return td.sendRequest(req)
}

func (td *testDriver) downloadAllGlobalServices(cluster string) []apis.GlobalService {
	req, _ := http.NewRequest(http.MethodGet, apiserver.PathGlobalServices, nil)
	req.Header.Add(apiserver.HeaderClusterName, cluster)

	resp := td.sendRequest(req)
	Expect(resp.Code).To(Equal(http.StatusOK))

	data, err := ioutil.ReadAll(resp.Body)
	Expect(err).To(BeNil())

	var services []apis.GlobalService
	Expect(json.Unmarshal(data, &services)).To(Succeed())

	return services
}

func (td *testDriver) sendRequest(req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	td.server.Handler.ServeHTTP(recorder, req)

	return recorder
}

func (td *testDriver) getService() apis.GlobalService {
	var svc apis.GlobalService

	err := k8sClient.Get(context.Background(), client.ObjectKey{Name: td.serviceName, Namespace: td.namespace}, &svc)
	Expect(err).To(Succeed())

	return svc
}

func (td *testDriver) ExpectServiceNotFound() {
	var svc apis.GlobalService

	err := k8sClient.Get(context.Background(), client.ObjectKey{Name: td.serviceName, Namespace: td.namespace}, &svc)
	Expect(errors.IsNotFound(err)).To(BeTrue())
}

func (td *testDriver) clearServices() {
	var services apis.GlobalServiceList
	Expect(k8sClient.List(context.Background(), &services)).To(Succeed())

	for _, svc := range services.Items {
		Expect(k8sClient.Delete(context.Background(), &svc)).To(Succeed())
	}
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
