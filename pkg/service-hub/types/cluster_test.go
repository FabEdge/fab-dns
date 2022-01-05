package types_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

var _ = Describe("ClusterStore", func() {
	var store *types.ClusterStore

	BeforeEach(func() {
		store = types.NewClusterStore()
	})

	It("New method will ensure only one cluster with given name created", func() {
		const clusterName = "test"
		c1 := store.New(clusterName)
		Expect(c1.Name()).To(Equal(clusterName))

		c2 := store.New(clusterName)
		Expect(c1).To(Equal(c2))
	})

	It("GetExpiredClusters will return expired clusters", func() {
		store.New("c1")
		expiredCluster := store.New("c2")
		expiredCluster.SetExpireTime(time.Now().Add(20 * time.Millisecond))

		time.Sleep(20 * time.Millisecond)

		clusters := store.GetExpiredClusters()
		Expect(len(clusters)).To(Equal(1))
		Expect(clusters[0]).To(Equal(expiredCluster))
	})
})
