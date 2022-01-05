package namespace

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fabedge/fab-dns/pkg/constants"
)

func Ensure(ctx context.Context, cli client.Client, name string) error {
	exists, err := Exists(ctx, cli, name)
	if err != nil {
		return err
	} else if exists {
		return nil
	}

	err = cli.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				constants.KeyCreatedBy: constants.AppServiceHub,
			},
		},
	})
	if errors.IsAlreadyExists(err) {
		err = nil
	}

	return err
}

func Exists(ctx context.Context, cli client.Client, name string) (bool, error) {
	err := cli.Get(ctx, client.ObjectKey{Name: name}, &corev1.Namespace{})
	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}
