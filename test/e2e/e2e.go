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
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/test/e2e/framework"
)

const (
	appNetTool    = "fabdns-net-tool"
	testNamespace = "fabdns-e2e-test"

	debugTool   = "debug-tool"
	deployment  = "nginx"
	statefulSet = "mysql"
	replicas    = 2

	labelKeyApp           = "app"
	labelKeyInstance      = "instance"
	labelKeyGlobalService = "fabedge.io/global-service"
)

var (
	serviceCloudClusterIP = "nginx"
	serviceCloudHeadless  = "mysql"

	// 标记是否有失败的spec
	hasFailedSpec = false

	clusters = make([]Cluster, 0)
)

func init() {
	_ = apis.AddToScheme(scheme.Scheme)

	rand.Seed(int64(time.Now().UnixNano()))
	serviceCloudClusterIP = getName(serviceCloudClusterIP)
	serviceCloudHeadless = getName(serviceCloudHeadless)
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
	configDir := framework.TestContext.MultiClusterConfigDir
	filelist, err := ioutil.ReadDir(configDir)
	if err != nil {
		framework.Failf("Error reading kubeconfig dir: %v", err)
	}
	clusterIPs := make([]string, 0)
	for _, f := range filelist {
		ipStr := f.Name()
		clusterIPs = append(clusterIPs, ipStr)
	}
	if len(clusterIPs) <= 1 {
		framework.Failf("Error no multi cluster condition, cluster IP list: %v", clusterIPs)
	}
	framework.Logf("kubeconfigDir=%v get cluster IP list: %v", configDir, clusterIPs)

	clusterNameList := []string{}
	for _, clusterIP := range clusterIPs {
		cluster, err := generateCluster(configDir, clusterIP)
		if err != nil {
			framework.Logf("Error generating cluster <%s> err: %v", clusterIP, err)
			continue
		}

		clusterNameList = append(clusterNameList, cluster.name+":"+clusterIP)
		clusters = append(clusters, cluster)
	}

	if len(clusterNameList) <= 1 {
		framework.Failf("Error no multi cluster condition, cluster list: %v", clusterNameList)
	}

	framework.Logf("cluster list: %v", clusterNameList)

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
		cluster.prepareStatefulSet(statefulSet, namespace, replicas)
		cluster.prepareDeployment(deployment, namespace, replicas)
		cluster.prepareDebugPod(debugTool, namespace)
	}
}

func prepareServicesOnEachCluster(namespace string) {
	for _, cluster := range clusters {
		cluster.prepareService(serviceCloudClusterIP, namespace, false)
		cluster.prepareService(serviceCloudHeadless, namespace, true)
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

func generateExpectedGlobalServices() map[string]apis.GlobalService {
	globalServices := make(map[string]apis.GlobalService)
	g := generateGlobalService(serviceCloudClusterIP, testNamespace, apis.ClusterIP)
	globalServices[serviceCloudClusterIP] = g

	g = generateGlobalService(serviceCloudHeadless, testNamespace, apis.Headless)
	globalServices[serviceCloudHeadless] = g

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
			framework.Failf("clusters exist not ready global services")
		}
	}
	framework.Logf("all cluster global services ready")
}

func equalEndpoints(a, b apis.Endpoint) bool {
	if a.Hostname != nil && b.Hostname != nil {
		if *(a.Hostname) != *(b.Hostname) {
			return false
		}
	} else if a.Hostname != b.Hostname {
		return false
	}
	switch {
	case a.Cluster != b.Cluster:
		return false
	case a.Zone != b.Zone:
		return false
	case a.Region != b.Region:
		return false
	case !reflect.DeepEqual(a.Addresses, b.Addresses):
		return false
	}
	return true
}

func createObject(cli client.Client, object client.Object) {
	framework.ExpectNoError(cli.Create(context.TODO(), object))
	framework.AddCleanupAction(func() {
		if err := cli.Delete(context.TODO(), object); err != nil {
			klog.Errorf("Failed to delete object %s, please delete it manually. Err: %s", object.GetName(), err)
		}
	})
}
