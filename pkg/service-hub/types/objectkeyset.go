package types

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Empty struct{}
type ObjectKeySet map[client.ObjectKey]Empty

func NewObjectKeySet() ObjectKeySet {
	return make(ObjectKeySet)
}

func (set ObjectKeySet) Add(key client.ObjectKey) {
	set[key] = Empty{}
}

func (set ObjectKeySet) Delete(key client.ObjectKey) {
	delete(set, key)
}

func (set ObjectKeySet) Has(key client.ObjectKey) bool {
	_, exists := set[key]

	return exists
}
