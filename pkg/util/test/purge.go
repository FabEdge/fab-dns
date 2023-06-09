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

package test

import (
	"context"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

func PurgeAllGlobalServices(cli client.Client, opts ...client.ListOption) {
	var gss apis.GlobalServiceList
	Expect(cli.List(context.TODO(), &gss)).To(Succeed())

	for _, gs := range gss.Items {
		Expect(cli.Delete(context.TODO(), &gs)).To(Succeed())
	}
}

func PurgeAllServices(cli client.Client, opts ...client.ListOption) {
	var services corev1.ServiceList
	Expect(cli.List(context.TODO(), &services)).To(Succeed())

	for _, obj := range services.Items {
		Expect(cli.Delete(context.TODO(), &obj)).To(Succeed())
	}
}

func PurgeAllEndpointSlices(cli client.Client, opts ...client.ListOption) {
	var endpointslices discoveryv1.EndpointSliceList
	Expect(cli.List(context.TODO(), &endpointslices)).To(Succeed())

	for _, obj := range endpointslices.Items {
		Expect(cli.Delete(context.TODO(), &obj)).To(Succeed())
	}
}
