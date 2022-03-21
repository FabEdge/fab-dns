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
	"fmt"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/fabedge/fab-dns/test/e2e/framework"
)

func podSpecWithAffinity() corev1.PodSpec {
	return corev1.PodSpec{
		// workaround, or it will fail at edgecore
		AutomountServiceAccountToken: new(bool),
		Containers: []corev1.Container{
			{
				Name:            "net-tool",
				Image:           framework.TestContext.NetToolImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 80,
					},
				},
			},
		},
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      framework.TestContext.EdgeLabels,
									Operator: corev1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
		},
	}
}

func getName(prefix string) string {
	time.Sleep(time.Millisecond)
	return fmt.Sprintf("%s-%d", prefix, rand.Int31n(1000))
}
