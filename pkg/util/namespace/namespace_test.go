package namespace_test

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nsutil "github.com/FabEdge/fab-dns/pkg/util/namespace"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace", func() {
	Describe("Ensure", func() {
		It("create a namespace if it doesn't exist", func() {
			namespace := "test"
			Expect(nsutil.Ensure(context.Background(), k8sClient, namespace)).To(Succeed())
			expectNamespaceExists(namespace)

			err := k8sClient.Delete(context.Background(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			})
			Expect(err).To(BeNil())
		})

		It("won't return error if namespace exists", func() {
			namespace := "default"
			expectNamespaceExists(namespace)

			Expect(nsutil.Ensure(context.Background(), k8sClient, namespace)).To(Succeed())
		})
	})
})

func expectNamespaceExists(name string) {
	exists, err := nsutil.Exists(context.Background(), k8sClient, name)
	Expect(err).To(BeNil())
	Expect(exists).To(BeTrue())
}
