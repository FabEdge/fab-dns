package e2e

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

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fabedge/fab-dns/test/e2e/framework"
)

// 测试集群内debug-pod访问集群内和集群间云端服务端点的情况
var _ = Describe("FabDNS", func() {
	It("the cloud debug-pod can access ClusterIP global service in cluster [fabdns]", func() {
		for _, cluster := range clusters {
			debugPod := corev1.Pod{}
			err := cluster.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: debugTool}, &debugPod)
			framework.ExpectNoError(err)

			serviceName := fmt.Sprintf("%s.%s.svc.%s", serviceCloudClusterIP, testNamespace, framework.TestContext.FabdnsZone)
			framework.Logf("pod %s of cluster %s visit global service %s ", debugTool, cluster.name, serviceName)
			_, _, err = cluster.execCurl(debugPod, serviceName)
			framework.ExpectNoError(err)
		}
	})

	It("the cloud debug-pod can access Headless global service in cluster [fabdns]", func() {
		for _, cluster := range clusters {
			debugPod := corev1.Pod{}
			err := cluster.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: debugTool}, &debugPod)
			framework.ExpectNoError(err)

			serviceName := fmt.Sprintf("%s.%s.svc.%s", serviceCloudHeadless, testNamespace, framework.TestContext.FabdnsZone)
			framework.Logf("pod %s of cluster %s visit global service %s ", debugTool, cluster.name, serviceName)
			_, _, err = cluster.execCurl(debugPod, serviceName)
			framework.ExpectNoError(err)
		}
	})

	It("the cloud debug-pod can access each endpoint of Headless global service in cluster [fabdns]", func() {
		for _, c1 := range clusters {
			debugPod := corev1.Pod{}
			err := c1.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: debugTool}, &debugPod)
			framework.ExpectNoError(err)

			for _, c2 := range clusters {
				for x := 0; x < replicas; x++ {
					hostname := fmt.Sprintf("%s-%d", statefulSet, x)
					serviceName := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname, c2.name,
						serviceCloudHeadless, testNamespace, framework.TestContext.FabdnsZone)
					framework.Logf("pod %s of cluster %s visit endpoint %s", debugTool, c1.name, serviceName)
					_, _, err := c1.execCurl(debugPod, serviceName)
					framework.ExpectNoError(err)
				}
			}
		}
	})
})
