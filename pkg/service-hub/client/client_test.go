package client_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/service-hub/apiserver"
	"github.com/FabEdge/fab-dns/pkg/service-hub/client"
)

var _ = Describe("Client", func() {
	const (
		clusterName = "fabedge"
	)
	var (
		mux    *http.ServeMux
		server *httptest.Server
		cli    client.Interface
	)

	BeforeEach(func() {
		mux = http.NewServeMux()
		server = httptest.NewServer(mux)

		var err error
		cli, err = client.NewClient(server.URL, clusterName, nil)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		server.Close()
	})

	It("can send heartbeat to API server", func() {
		var req *http.Request
		mux.HandleFunc(apiserver.PathHeartbeat, func(w http.ResponseWriter, r *http.Request) {
			req = r

			w.WriteHeader(http.StatusNoContent)
		})

		Expect(cli.Heartbeat()).To(Succeed())
		Expect(req.Header.Get(apiserver.HeaderClusterName)).To(Equal(clusterName))
		Expect(req.Method).To(Equal(http.MethodGet))
	})

	It("can upload a global service to API server", func() {
		var req *http.Request
		var receivedContent []byte
		mux.HandleFunc(apiserver.PathGlobalServices, func(w http.ResponseWriter, r *http.Request) {
			req = r
			receivedContent, _ = ioutil.ReadAll(r.Body)

			w.WriteHeader(http.StatusNoContent)
		})

		service := apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: apis.GlobalServiceSpec{
				Ports: []apis.ServicePort{
					{
						Name:     "web",
						Port:     80,
						Protocol: corev1.ProtocolTCP,
					},
				},
				Endpoints: []apis.Endpoint{
					{
						Addresses: []string{"192.168.1.1"},
					},
				},
			},
		}
		expectedContent, _ := json.Marshal(service)

		Expect(cli.UploadGlobalService(service)).To(Succeed())
		Expect(req.Header.Get(apiserver.HeaderClusterName)).To(Equal(clusterName))
		Expect(req.Method).To(Equal(http.MethodPost))
		Expect(receivedContent).To(Equal(expectedContent))
	})

	It("can download all global services API server", func() {
		expectedServices := []apis.GlobalService{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: apis.GlobalServiceSpec{
					Ports: []apis.ServicePort{
						{
							Name:     "web",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Endpoints: []apis.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "test",
				},
				Spec: apis.GlobalServiceSpec{
					Ports: []apis.ServicePort{
						{
							Name:     "web",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Endpoints: []apis.Endpoint{
						{
							Addresses: []string{"192.168.1.2"},
						},
					},
				},
			},
		}

		var req *http.Request
		mux.HandleFunc(apiserver.PathGlobalServices, func(w http.ResponseWriter, r *http.Request) {
			req = r
			data, _ := json.Marshal(expectedServices)
			w.Write(data)
		})

		services, err := cli.DownloadAllGlobalServices()
		Expect(err).To(BeNil())
		Expect(req.Header.Get(apiserver.HeaderClusterName)).To(Equal(clusterName))
		Expect(req.Method).To(Equal(http.MethodGet))
		Expect(services).To(Equal(expectedServices))
	})

	It("can delete a global service from API server", func() {
		var (
			namespace   = "default"
			serviceName = "nginx"
		)
		var req *http.Request
		var expectedPath = fmt.Sprintf("%s/%s/%s", apiserver.PathGlobalServices, namespace, serviceName)
		mux.HandleFunc(expectedPath, func(w http.ResponseWriter, r *http.Request) {
			req = r
			w.WriteHeader(http.StatusNoContent)
		})

		Expect(cli.DeleteGlobalService(namespace, serviceName)).To(Succeed())
		Expect(req.URL.Path).To(Equal(expectedPath))
		Expect(req.Header.Get(apiserver.HeaderClusterName)).To(Equal(clusterName))
		Expect(req.Method).To(Equal(http.MethodDelete))
	})
})
