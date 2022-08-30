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

package summary

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	installinformers "github.com/daocloud/karmada-operator/pkg/generated/informers/externalversions/install/v1alpha1"
	installliter "github.com/daocloud/karmada-operator/pkg/generated/listers/install/v1alpha1"
)

const (
	StatusControllerFinalizer = "karmada.install.io/summary-controller"
	// DefaultJobBackOff is the default backoff period. Exported for tests.
	DefaultKmdBackOff = 2 * time.Second
	// MaxJobBackOff is the max backoff period. Exported for tests.
	MaxKmdBackOff = 5 * time.Second
)

type SummaryController struct {
	sync.RWMutex
	stopCh    <-chan struct{}
	client    clientset.Interface
	kmdClient versioned.Interface

	queue        workqueue.RateLimitingInterface
	installStore installliter.KarmadaDeploymentLister
	// instalStoreSynced returns true if the kmd store has been synced at least once.
	// Added as a member to the struct to allow injection for testing.
	instalStoreSynced cache.InformerSynced
	// To allow injection of syncKmdStatusResource for testing.
	syncHandler func(kmdKey string) (bool, error)
	managers    map[string]*SummaryManager
	waitGroup   wait.Group
}

func NewController(client clientset.Interface, kmdClient versioned.Interface, kmdInformer installinformers.KarmadaDeploymentInformer) *SummaryController {
	controller := &SummaryController{
		client:    client,
		kmdClient: kmdClient,
		queue: workqueue.NewRateLimitingQueue(
			workqueue.NewItemExponentialFailureRateLimiter(DefaultKmdBackOff, MaxKmdBackOff),
		),
		managers: make(map[string]*SummaryManager),
	}

	kmdInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.AddEvent,
			UpdateFunc: controller.UpdateEventfunc,
			DeleteFunc: controller.DeleteEvent,
		},
	)
	controller.installStore = kmdInformer.Lister()
	controller.instalStoreSynced = kmdInformer.Informer().HasSynced
	controller.syncHandler = controller.syncKmdStatusResource

	return controller
}

func (rc *SummaryController) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	rc.queue.Add(key)
}

func (rc *SummaryController) AddEvent(obj interface{}) {
	kmd := obj.(*installv1alpha1.KarmadaDeployment)
	if !kmd.Status.ControlPlaneReady {
		return
	}

	rc.enqueue(kmd)
}

func (rc *SummaryController) DeleteEvent(obj interface{}) {
	rc.enqueue(obj)
}

// UpdateEventfunc only karmadaDeployment ControlPlaneReady is true
// or secretRef had changed, we will add queue.
func (rc *SummaryController) UpdateEventfunc(older, newer interface{}) {
	oldObj := older.(*installv1alpha1.KarmadaDeployment)
	newObj := newer.(*installv1alpha1.KarmadaDeployment)
	if !newObj.Status.ControlPlaneReady {
		return
	}
	if newObj.DeletionTimestamp.IsZero() &&
		equality.Semantic.DeepEqual(oldObj.Status.SecretRef, newObj.Status.SecretRef) {
		return
	}
	rc.enqueue(newer)
}

func (rc *SummaryController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer rc.queue.ShutDown()

	klog.InfoS("Start karmadaDeployment summary controller")
	defer klog.InfoS("Shutting down karmadaDeployment summary controller")

	if rc.stopCh != nil {
		return
	}
	rc.stopCh = stopCh

	if !cache.WaitForCacheSync(stopCh, rc.instalStoreSynced) {
		return
	}

	klog.InfoS("kmd summary controller is running", "workers", workers)
	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			wait.Until(rc.worker, time.Second, rc.stopCh)
		}()
	}

	<-stopCh

	klog.InfoS("wait for cluster synchros stop...")
	rc.waitGroup.Wait()
	klog.InfoS("cluster synchro manager stoped.")
}

func (rc *SummaryController) worker() {
	for rc.processNextWorkItem() {
		select {
		case <-rc.stopCh:
			return
		default:
		}
	}
}

func (rc *SummaryController) processNextWorkItem() bool {
	key, quit := rc.queue.Get()
	if quit {
		return false
	}
	defer rc.queue.Done(key)

	forget, err := rc.syncHandler(key.(string))
	if err == nil {
		if forget {
			rc.queue.Forget(key)
		}
		return true
	}

	utilruntime.HandleError(fmt.Errorf("sync %q failed with %v", key, err))
	rc.queue.AddRateLimited(key)

	return true
}

func (rc *SummaryController) syncKmdStatusResource(key string) (bool, error) {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing %q (%v)", key, time.Since(startTime))
	}()

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return false, err
	}

	kmd, err := rc.installStore.Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("%v has been deleted", key)
			return true, nil
		}
		return false, err
	}

	if !kmd.DeletionTimestamp.IsZero() {
		klog.InfoS("remove karmadaDeployment", "karmadaDeployment", kmd.Name)
		if err := rc.removekarmadaDeployment(kmd); err != nil {
			klog.InfoS("failed to remove karmadaDeployment", "karmadaDeployment", kmd.Name)
			return false, nil
		}

		controllerutil.ContainsFinalizer(kmd.DeepCopy(), StatusControllerFinalizer)
		_ = controllerutil.RemoveFinalizer(kmd, StatusControllerFinalizer)
		if _, err := rc.kmdClient.InstallV1alpha1().KarmadaDeployments().Update(context.TODO(), kmd, metav1.UpdateOptions{}); err != nil {
			return false, err
		}

		return true, nil
	}
	// ensure finalizer
	if !controllerutil.ContainsFinalizer(kmd, StatusControllerFinalizer) {
		_ = controllerutil.AddFinalizer(kmd, StatusControllerFinalizer)
		if kmd, err = rc.kmdClient.InstallV1alpha1().KarmadaDeployments().Update(context.TODO(), kmd, metav1.UpdateOptions{}); err != nil {
			return false, err
		}
	}

	manager, err := rc.GetSummaryManager(kmd)
	if err != nil {
		return false, err
	}
	rc.waitGroup.StartWithChannel(rc.stopCh, manager.Run)

	return true, nil
}

// removekarmadaDeployment shut down manager of the kmd as soon as remove it.
// and then remove the manager from the list of manager.
func (rc *SummaryController) removekarmadaDeployment(kmd *installv1alpha1.KarmadaDeployment) error {
	rc.Lock()
	defer rc.Unlock()
	manager := rc.managers[kmd.Name]
	if manager == nil {
		return nil
	}

	manager.Shuntdown()

	delete(rc.managers, kmd.Name)
	return nil
}

// GetSummaryManager get a manager form list. it will create manager for kmd if it's not in list.
func (rc *SummaryController) GetSummaryManager(kmd *installv1alpha1.KarmadaDeployment) (*SummaryManager, error) {
	manager := func() *SummaryManager {
		rc.RLock()
		defer rc.RUnlock()
		return rc.managers[kmd.Name]
	}()

	if manager != nil {
		return manager, nil
	}

	rc.Lock()
	defer rc.Unlock()

	manager, exist := rc.managers[kmd.Name]
	if exist {
		return manager, nil
	}
	dynamicClient, err := NewDynamicClientFormSecretRef(rc.client, kmd.Status.SecretRef)
	if err != nil {
		return nil, err
	}

	manager = NewSummaryManager(kmd.Name, rc.kmdClient, rc.installStore, dynamicClient)
	rc.managers[kmd.Name] = manager

	return manager, nil
}

func NewDynamicClientFormSecretRef(client clientset.Interface, secretRef *installv1alpha1.LocalSecretReference) (dynamic.Interface, error) {
	secret, err := client.CoreV1().Secrets(secretRef.Namespace).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	kubeconfig := secret.Data["kubeconfig"]
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(config)
}
