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

var _ = Describe("FabDNS", func() {
	It("any pod can access ClusterIP global service by domain name", func() {
		for _, cluster := range clusters {
			var clientPod corev1.Pod
			err := cluster.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: nameNetTool}, &clientPod)
			framework.ExpectNoError(err)

			url := fmt.Sprintf("%s.%s.svc.%s", serviceNameNginx, testNamespace, framework.TestContext.FabdnsZone)

			framework.Logf("%s/%s visit global service %s ", cluster.name, nameNetTool, url)
			_, _, err = cluster.execCurl(clientPod, url)
			framework.ExpectNoError(err)

			if framework.TestContext.IPv6Enabled {
				framework.Logf("%s/%s visit global service %s using IPv6", cluster.name, nameNetTool, url)
				_, _, err = cluster.execCurl(clientPod, url)
				framework.ExpectNoError(err)
			}
		}
	})

	It("any pod can access Headless global service by domain name", func() {
		for _, cluster := range clusters {
			var clientPod corev1.Pod
			err := cluster.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: nameNetTool}, &clientPod)
			framework.ExpectNoError(err)

			url := fmt.Sprintf("%s.%s.svc.%s", serviceNameMySQL, testNamespace, framework.TestContext.FabdnsZone)

			framework.Logf("%s/%s visit global service %s ", cluster.name, nameNetTool, url)
			_, _, err = cluster.execCurl(clientPod, url)
			framework.ExpectNoError(err)

			if framework.TestContext.IPv6Enabled {
				framework.Logf("%s/%s visit global service %s using IPv6", cluster.name, nameNetTool, url)
				_, _, err = cluster.execCurl6(clientPod, url)
				framework.ExpectNoError(err)
			}
		}
	})

	It("any pod can access each endpoint of Headless global service by domain name", func() {
		for _, c1 := range clusters {
			clientPod := corev1.Pod{}
			err := c1.client.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: nameNetTool}, &clientPod)
			framework.ExpectNoError(err)

			for _, c2 := range clusters {
				for x := 0; x < 2; x++ {
					hostname := fmt.Sprintf("%s-%d", nameMySQL, x)

					url := fmt.Sprintf("%s.%s.%s.%s.svc.%s", hostname, c2.name,
						serviceNameMySQL, testNamespace, framework.TestContext.FabdnsZone)

					framework.Logf("%s/%s visit %s ", c1.name, nameNetTool, url)
					_, _, err := c1.execCurl(clientPod, url)
					framework.ExpectNoError(err)

					if framework.TestContext.IPv6Enabled {
						framework.Logf("%s/%s visit %s using IPv6", c1.name, nameNetTool, url)
						_, _, err = c1.execCurl6(clientPod, url)
						framework.ExpectNoError(err)
					}
				}
			}
		}
	})
})
