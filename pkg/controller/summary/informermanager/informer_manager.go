/*
Copyright 2022 The Karmada operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package informermanager

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/daocloud/karmada-operator/pkg/utils"
)

// InformerManager manages dynamic shared informer for all resources, include Kubernetes resource and
// custom resources defined by CustomResourceDefinition.
type InformerManager interface {
	// ForResource builds a dynamic shared informer for 'resource' then set event handler.
	// If the informer already exist, the event handler will be appended to the informer.
	// The handler should not be nil.
	ForResource(resource schema.GroupVersionResource, handler cache.ResourceEventHandler)

	// IsInformerSynced checks if the resource's informer is synced.
	// An informer is synced means:
	// - The informer has been created(by method 'ForResource' or 'Lister').
	// - The informer has started(by method 'Start').
	// - The informer's cache has been synced.
	IsInformerSynced(resource schema.GroupVersionResource) bool

	// IsHandlerExist checks if handler already added to the informer that watches the 'resource'.
	IsHandlerExist(resource schema.GroupVersionResource, handler cache.ResourceEventHandler) bool

	// Lister returns a generic lister used to get 'resource' from informer's store.
	// The informer for 'resource' will be created if not exist, but without any event handler.
	Lister(resource schema.GroupVersionResource) cache.GenericLister

	// Start will run all informers, the informers will keep running until the channel closed.
	// It is intended to be called after create new informer(s), and it's safe to call multi times.
	Start()

	// Stop stops all single cluster informers of a cluster. Once it is stopped, it will be not able
	// to Start again.
	Stop()

	// WaitForCacheSync waits for all caches to populate.
	WaitForCacheSync() map[schema.GroupVersionResource]bool

	// WaitForCacheSyncWithTimeout waits for all caches to populate with a definitive timeout.
	WaitForCacheSyncWithTimeout(cacheSyncTimeout time.Duration) map[schema.GroupVersionResource]bool

	// Context returns the single cluster context.
	Context() context.Context

	// GetClient returns the dynamic client.
	GetClient() dynamic.Interface
}

// NewInformerManager constructs a new instance of InformerManagerImpl.
// defaultResync with value '0' means no re-sync.
func NewInformerManager(client dynamic.Interface, defaultResync time.Duration, parentCh <-chan struct{}) InformerManager {
	ctx, cancel := utils.ContextForChannel(parentCh)
	return &informerManagerImpl{
		informerFactory: dynamicinformer.NewDynamicSharedInformerFactory(client, defaultResync),
		handlers:        make(map[schema.GroupVersionResource][]cache.ResourceEventHandler),
		syncedInformers: make(map[schema.GroupVersionResource]struct{}),
		ctx:             ctx,
		cancel:          cancel,
		client:          client,
	}
}

type informerManagerImpl struct {
	ctx    context.Context
	cancel context.CancelFunc

	informerFactory dynamicinformer.DynamicSharedInformerFactory

	syncedInformers map[schema.GroupVersionResource]struct{}

	handlers map[schema.GroupVersionResource][]cache.ResourceEventHandler

	client dynamic.Interface

	lock sync.RWMutex
}

func (s *informerManagerImpl) ForResource(resource schema.GroupVersionResource, handler cache.ResourceEventHandler) {
	// if handler already exist, just return, nothing changed.
	if s.IsHandlerExist(resource, handler) {
		return
	}

	s.informerFactory.ForResource(resource).Informer().AddEventHandler(handler)
	s.appendHandler(resource, handler)
}

func (s *informerManagerImpl) IsInformerSynced(resource schema.GroupVersionResource) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, exist := s.syncedInformers[resource]
	return exist
}

func (s *informerManagerImpl) IsHandlerExist(resource schema.GroupVersionResource, handler cache.ResourceEventHandler) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	handlers, exist := s.handlers[resource]
	if !exist {
		return false
	}

	for _, h := range handlers {
		if h == handler {
			return true
		}
	}

	return false
}

func (s *informerManagerImpl) Lister(resource schema.GroupVersionResource) cache.GenericLister {
	return s.informerFactory.ForResource(resource).Lister()
}

func (s *informerManagerImpl) appendHandler(resource schema.GroupVersionResource, handler cache.ResourceEventHandler) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// assume the handler list exist, caller should ensure for that.
	handlers := s.handlers[resource]

	// assume the handler not exist in it, caller should ensure for that.
	s.handlers[resource] = append(handlers, handler)
}

func (s *informerManagerImpl) Start() {
	s.informerFactory.Start(s.ctx.Done())
}

func (s *informerManagerImpl) Stop() {
	s.cancel()
}

func (s *informerManagerImpl) WaitForCacheSync() map[schema.GroupVersionResource]bool {
	return s.waitForCacheSync(s.ctx)
}

func (s *informerManagerImpl) WaitForCacheSyncWithTimeout(cacheSyncTimeout time.Duration) map[schema.GroupVersionResource]bool {
	ctx, cancel := context.WithTimeout(s.ctx, cacheSyncTimeout)
	defer cancel()

	return s.waitForCacheSync(ctx)
}

func (s *informerManagerImpl) waitForCacheSync(ctx context.Context) map[schema.GroupVersionResource]bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	res := s.informerFactory.WaitForCacheSync(ctx.Done())
	for resource, synced := range res {
		if _, exist := s.syncedInformers[resource]; !exist && synced {
			s.syncedInformers[resource] = struct{}{}
		}
	}
	return res
}

func (s *informerManagerImpl) Context() context.Context {
	return s.ctx
}

func (s *informerManagerImpl) GetClient() dynamic.Interface {
	return s.client
}
