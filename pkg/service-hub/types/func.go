package types

import (
	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

type ExportGlobalServiceFunc func(service apis.GlobalService) error
type RevokeGlobalServiceFunc func(clusterName, namespace, serviceName string) error
