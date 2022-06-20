package types

import (
	"context"
	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

type ExportGlobalServiceFunc func(ctx context.Context, service apis.GlobalService) error
type RevokeGlobalServiceFunc func(ctx context.Context, clusterName, namespace, serviceName string) error
