package namespace_test

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	testutil "github.com/FabEdge/fab-dns/pkg/util/test"
)

var kubeConfig *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

var _ = BeforeSuite(func(done Done) {
	testutil.SetupLogger()

	By("starting test environment")
	var err error
	testEnv, kubeConfig, k8sClient, err = testutil.StartTestEnvWithCRD(
		[]string{filepath.Join("..", "..", "deploy", "crd")},
	)
	Expect(err).ToNot(HaveOccurred())

	_ = apis.AddToScheme(scheme.Scheme)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ShouldNot(HaveOccurred())
})

func TestNamespace(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Namespace Suite")
}
