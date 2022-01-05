package apiserver_test

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	testutil "github.com/fabedge/fab-dns/pkg/util/test"
)

var k8sClient client.Client
var testEnv *envtest.Environment

var _ = BeforeSuite(func(done Done) {
	testutil.SetupLogger()

	By("starting test environment")
	var err error
	testEnv, _, k8sClient, err = testutil.StartTestEnvWithCRD(
		[]string{filepath.Join("..", "..", "..", "deploy", "crd")},
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

func TestAPIServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "APIServer Suite")
}
