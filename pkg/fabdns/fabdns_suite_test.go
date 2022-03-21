package fabdns

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	testutil "github.com/fabedge/fab-dns/pkg/util/test"
)

var testCfg *rest.Config
var testK8sClient client.Client

// envtest provide an api server which has some differences from real environments,
// read https://book.kubebuilder.io/reference/envtest.html#testing-considerations
var testEnv *envtest.Environment

var _ = BeforeSuite(func(done Done) {
	testutil.SetupLogger()

	By("starting test environment")
	var err error
	testEnv, testCfg, testK8sClient, err = testutil.StartTestEnvWithCRD(
		[]string{filepath.Join("..", "..", "deploy", "crd")},
	)
	Expect(err).ToNot(HaveOccurred())

	_ = apis.AddToScheme(scheme.Scheme)

	buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
		return testCfg, nil
	}

	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	buildConfigFromFlags = clientcmd.BuildConfigFromFlags
	err := testEnv.Stop()
	Expect(err).ShouldNot(HaveOccurred())
})

func TestFabdns(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fabdns Suite")
}
