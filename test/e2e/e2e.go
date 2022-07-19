// Copyright 2021 FabEdge Team
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"io/ioutil"
	"math/rand"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/test/e2e/framework"
)

const (
	testNamespace = "fabdns-e2e-test"

	nameNetTool = "net-tool"
	nameNginx   = "nginx"
	nameMySQL   = "mysql"

	labelKeyApp           = "app"
	labelKeyGlobalService = "fabedge.io/global-service"
)

var (
	serviceNameNginx = "nginx"
	//serviceNameNginx6 = "nginx6"
	serviceNameMySQL = "mysql"
	//serviceNameMySQL6 = "mysql6"

	// 标记是否有失败的spec
	hasFailedSpec = false

	clusters = make([]Cluster, 0)
)

func init() {
	_ = apis.AddToScheme(scheme.Scheme)

	rand.Seed(int64(time.Now().UnixNano()))
	serviceNameNginx = getName(serviceNameNginx)
	//serviceNameNginx6 = getName(serviceNameNginx6)
	serviceNameMySQL = getName(serviceNameMySQL)
	//serviceNameMySQL6 = getName(serviceNameMySQL6)
}

// RunE2ETests checks configuration parameters (specified through flags) and then runs
// E2E tests using the Ginkgo runner.
func RunE2ETests(t *testing.T) {
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		hasFailedSpec = true
		ginkgo.Fail(message, callerSkip...)
	})

	if framework.TestContext.GenReport {
		reportFile := framework.TestContext.ReportFile
		framework.Logf("test report will be written to file %s", reportFile)
		ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "FabDNS Network Tests", []ginkgo.Reporter{
			framework.NewTableReporter(reportFile),
		})
	} else {
		ginkgo.RunSpecs(t, "FabDNS Network Tests")
	}
}

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	fabdnsE2eTestPrepare()
	return nil
}, func(_ []byte) {
})

var _ = ginkgo.SynchronizedAfterSuite(func() {
	framework.Logf("fabdns test suite finished")
	switch framework.PreserveResourcesMode(framework.TestContext.PreserveResources) {
	case framework.PreserveResourcesAlways:
		framework.Logf("resources are preserved, please prune them manually before next time")
	case framework.PreserveResourcesOnFailure:
		if hasFailedSpec {
			framework.Logf("resources are preserved as some specs failed, please prune them manually before next time")
			return
		}
		fallthrough
	case framework.PreserveResourcesNever:
		framework.Logf("pruning resources")
		framework.RunCleanupActions()
	}
}, func() {
})

func fabdnsE2eTestPrepare() {
	framework.Logf("fabdns e2e test")
	// read dir get all cluster IPs
	configDir := framework.TestContext.KubeConfigsDir
	files, err := ioutil.ReadDir(configDir)
	if err != nil {
		framework.Failf("Error reading kubeconfig dir: %v", err)
	}

	if len(files) <= 1 {
		framework.Failf("only %d clusters are found, can not do e2e test", len(files))
	}

	for _, f := range files {
		kubeconfigPath := path.Join(configDir, f.Name())
		cluster, err := generateCluster(kubeconfigPath)
		if err != nil {
			framework.Logf("failed to create cluster from kubeconfig file %s. err: %v", kubeconfigPath, err)
			continue
		}

		clusters = append(clusters, cluster)
	}

	prepareClustersNamespace(testNamespace)
	preparePodsOnEachCluster(testNamespace)
	prepareServicesOnEachCluster(testNamespace)
	WaitForAllClusterPodsReady(testNamespace)

	WaitForAllClusterGlobalServicesReady(testNamespace)
}

func prepareClustersNamespace(namespace string) {
	for _, cluster := range clusters {
		cluster.prepareNamespace(namespace)
	}
}

func preparePodsOnEachCluster(namespace string) {
	framework.Logf("Prepare pods on each cluster")
	for _, cluster := range clusters {
		cluster.prepareMySQLStatefulSet(namespace)
		cluster.prepareNginxDeployment(namespace)
		cluster.prepareDebugPod(nameNetTool, namespace)
	}
}

func prepareServicesOnEachCluster(namespace string) {
	ipFamilies := []corev1.IPFamily{corev1.IPv4Protocol}
	if framework.TestContext.IPv6Enabled {
		ipFamilies = append(ipFamilies, corev1.IPv6Protocol)
	}
	for _, cluster := range clusters {
		cluster.prepareService(serviceNameNginx, namespace, nameNginx, false, ipFamilies)
		cluster.prepareService(serviceNameMySQL, namespace, nameMySQL, true, ipFamilies)
		//if framework.TestContext.IPv6Enabled {
		//	cluster.prepareService(serviceNameNginx6, namespace, nameNginx, false, corev1.IPv6Protocol)
		//	cluster.prepareService(serviceNameMySQL6, namespace, nameMySQL, true, corev1.IPv6Protocol)
		//}
	}
}

func WaitForAllClusterPodsReady(namespace string) {
	var wg sync.WaitGroup
	for i := range clusters {
		wg.Add(1)
		go clusters[i].waitForClusterPodsReady(&wg, namespace)
	}
	wg.Wait()

	for _, cluster := range clusters {
		if !cluster.podsReady {
			framework.Failf("clusters exist not ready pods")
		}
	}
}

func generateExpectedGlobalServices() []apis.GlobalService {
	globalServices := []apis.GlobalService{
		generateGlobalService(serviceNameNginx, testNamespace, apis.ClusterIP),
		generateGlobalService(serviceNameMySQL, testNamespace, apis.Headless),
	}

	//if framework.TestContext.IPv6Enabled {
	//	globalServices = append(globalServices,
	//		generateGlobalService(serviceNameNginx6, testNamespace, apis.ClusterIP),
	//		generateGlobalService(serviceNameMySQL6, testNamespace, apis.Headless),
	//	)
	//}

	return globalServices
}

func generateGlobalService(name, namespace string, serviceType apis.ServiceType) apis.GlobalService {
	g := apis.GlobalService{}
	g.Spec.Type = serviceType
	g.Name = name
	g.Namespace = namespace
	for _, cluster := range clusters {
		eps, err := cluster.generateGlobalServiceEndpoints(g.Name, g.Namespace, serviceType)
		if err != nil {
			framework.Failf("cluster %s failed to generate globalservice %s endpoints", cluster.name, g.Name)
		}
		g.Spec.Endpoints = append(g.Spec.Endpoints, eps...)
	}
	return g
}

func WaitForAllClusterGlobalServicesReady(namespace string) {
	expectedGlobalServices := generateExpectedGlobalServices()
	var wg sync.WaitGroup
	for i := range clusters {
		wg.Add(1)
		go clusters[i].waitForGlobalServicesReady(&wg, namespace, expectedGlobalServices)
	}
	wg.Wait()

	for _, cluster := range clusters {
		if !cluster.ready() {
			framework.Failf("cluster %s is not ready after %d seconds", cluster.name, framework.TestContext.WaitTimeout)
		}
	}
	framework.Logf("all cluster global services ready")
}

func createObject(cli client.Client, object client.Object) {
	framework.ExpectNoError(cli.Create(context.TODO(), object))
	framework.AddCleanupAction(func() {
		if err := cli.Delete(context.TODO(), object); err != nil {
			klog.Errorf("Failed to delete object %s, please delete it manually. Err: %s", object.GetName(), err)
		}
	})
}
