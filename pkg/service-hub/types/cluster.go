package types

import (
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Cluster is used to store all keys of global services reported by
// a cluster. A cluster should be created by ClusterStore.New method
type Cluster struct {
	name          string
	serviceKeySet ObjectKeySet
	expireTime    time.Time
	lock          sync.RWMutex
}

// ClusterStore is used to manage clusters and is thread-safe
type ClusterStore struct {
	lock     sync.RWMutex
	clusters map[string]*Cluster
}

func NewClusterStore() *ClusterStore {
	return &ClusterStore{
		clusters: make(map[string]*Cluster),
	}
}

func (c *Cluster) Name() string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.name
}

func (c *Cluster) ExpireTime() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.expireTime
}

func (c *Cluster) SetExpireTime(expireTime time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.expireTime = expireTime
}

func (c *Cluster) IsExpired() bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	return !c.expireTime.IsZero() && c.expireTime.Before(time.Now())
}

func (c *Cluster) GetAllServiceKeys() []client.ObjectKey {
	c.lock.Lock()
	defer c.lock.Unlock()

	var keys []client.ObjectKey
	for key := range c.serviceKeySet {
		keys = append(keys, key)
	}

	return keys
}

func (c *Cluster) AddServiceKey(key client.ObjectKey) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.serviceKeySet.Add(key)
}

func (c *Cluster) RemoveServiceKey(key client.ObjectKey) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.serviceKeySet.Delete(key)
}

func (store *ClusterStore) New(name string) *Cluster {
	store.lock.Lock()
	defer store.lock.Unlock()

	if _, exists := store.clusters[name]; !exists {
		store.clusters[name] = &Cluster{
			serviceKeySet: NewObjectKeySet(),
			name:          name,
		}
	}

	return store.clusters[name]
}

func (store *ClusterStore) Get(name string) *Cluster {
	store.lock.RLock()
	defer store.lock.RUnlock()

	return store.clusters[name]
}

func (store *ClusterStore) Remove(name string) {
	store.lock.Lock()
	defer store.lock.Unlock()

	delete(store.clusters, name)
}

func (store *ClusterStore) RemoveClusters(names ...string) {
	store.lock.Lock()
	defer store.lock.Unlock()

	for _, name := range names {
		delete(store.clusters, name)
	}
}

func (store *ClusterStore) GetExpiredClusters() []*Cluster {
	store.lock.RLock()
	defer store.lock.RUnlock()

	var clusters []*Cluster
	for _, c := range store.clusters {
		if c.IsExpired() {
			clusters = append(clusters, c)
		}
	}

	return clusters
}
