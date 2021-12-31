package test

import (
	"context"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	nsutil "github.com/FabEdge/fab-dns/pkg/util/namespace"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ExpectNamespaceExists(cli client.Client, name string) {
	exists, err := nsutil.Exists(context.Background(), cli, name)
	Expect(err).To(Succeed())
	Expect(exists).To(BeTrue())
}

func ExpectNamespaceNotExists(cli client.Client, name string) {
	exists, err := nsutil.Exists(context.Background(), cli, name)
	Expect(err).To(Succeed())
	Expect(exists).To(BeFalse())
}

func ExpectGlobalServiceNotFound(cli client.Client, key client.ObjectKey) {
	err := cli.Get(context.Background(), key, &apis.GlobalService{})
	Expect(errors.IsNotFound(err)).To(BeTrue())
}

func ExpectGetGlobalService(cli client.Client, key client.ObjectKey) apis.GlobalService {
	var svc apis.GlobalService

	err := cli.Get(context.Background(), key, &svc)
	Expect(err).To(Succeed())

	return svc
}
