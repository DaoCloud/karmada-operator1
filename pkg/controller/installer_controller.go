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

package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/constants"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	installinformers "github.com/daocloud/karmada-operator/pkg/generated/informers/externalversions/install/v1alpha1"
	installliter "github.com/daocloud/karmada-operator/pkg/generated/listers/install/v1alpha1"
	factory "github.com/daocloud/karmada-operator/pkg/installer"
	helminstaller "github.com/daocloud/karmada-operator/pkg/installer/helm"
)

const (
	// maximum retry times.
	MaxInstallSyncRetry = 30
	ControllerFinalizer = "karmada.install.io/installer-controller"
	// DisableCascadingDeletionLabel is the label that determine whether to perform cascade deletion
	DisableCascadingDeletionLabel = "karmada.install.io/disable-cascading-deletion"
)

type Controller struct {
	runLock sync.Mutex
	stopCh  <-chan struct{}

	clientset clientset.Interface
	kmdClient versioned.Interface
	queue     workqueue.RateLimitingInterface

	factory *factory.InstallerFactory

	installStore installliter.KarmadaDeploymentLister
	// instalStoreSynced returns true if the kmd store has been synced at least once.
	// Added as a member to the struct to allow injection for testing.
	instalStoreSynced cache.InformerSynced
}

func NewController(kmdClient versioned.Interface,
	client clientset.Interface,
	chartResource *helminstaller.ChartResource,
	karmadaDeploymentInformer installinformers.KarmadaDeploymentInformer,
) *Controller {
	controller := &Controller{
		kmdClient:         kmdClient,
		clientset:         client,
		installStore:      karmadaDeploymentInformer.Lister(),
		instalStoreSynced: karmadaDeploymentInformer.Informer().HasSynced,
		queue: workqueue.NewRateLimitingQueue(
			workqueue.NewItemExponentialFailureRateLimiter(2*time.Second, 5*time.Second),
		),

		factory: factory.NewInstallerFactory(kmdClient, client, chartResource),
	}

	karmadaDeploymentInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.AddEvent,
			UpdateFunc: controller.UpdateEvent,
			DeleteFunc: controller.DeleteEvent,
		},
	)

	return controller
}

func (c *Controller) UpdateEvent(older, newer interface{}) {
	oldObj := older.(*installv1alpha1.KarmadaDeployment)
	newObj := newer.(*installv1alpha1.KarmadaDeployment)
	if !newObj.DeletionTimestamp.IsZero() || !equality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) {
		c.enqueue(newer)
	}
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	c.queue.Add(key)
}

func (c *Controller) DeleteEvent(obj interface{}) {
	c.enqueue(obj)
}

func (c *Controller) AddEvent(obj interface{}) {
	c.enqueue(obj)
}

func (c *Controller) Run(workers int, stopCh <-chan struct{}) {
	c.runLock.Lock()
	defer c.runLock.Unlock()

	klog.InfoS("Start karmadaDeployment operator")
	defer klog.InfoS("Shutting down karmadaDeployment operator")

	if c.stopCh != nil {
		return
	}
	c.stopCh = stopCh

	if !cache.WaitForCacheSync(stopCh, c.instalStoreSynced) {
		return
	}

	klog.InfoS("Karmada Operator is running", "workers", workers)

	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			wait.Until(c.worker, time.Second, c.stopCh)
		}()
	}

	<-c.stopCh

	c.queue.ShutDown()
	waitGroup.Wait()
}

func (c *Controller) worker() {
	for c.processNext() {
		select {
		case <-c.stopCh:
			return
		default:
		}
	}
}

func (c *Controller) processNext() bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	name := key.(string)
	if err := c.syncHandler(name); err != nil {
		klog.ErrorS(err, "Failed to reconcile karmadaDeployment", "cluster", name, "num requeues", c.queue.NumRequeues(key))

		if c.queue.NumRequeues(key) < MaxInstallSyncRetry {
			c.queue.AddRateLimited(key)
			return true
		}
		klog.V(2).Infof("Dropping karmadaDeployment %q out of the queue: %v", key, err)
	}

	c.queue.Forget(key)
	return true
}

func (c *Controller) syncHandler(key string) (err error) {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing %q (%v)", key, time.Since(startTime))
	}()

	// KarmadaDeployment is cluster scope, key == name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "failed to split karmadaDeployment key", "key", key)
		return err
	}
	kmd, err := c.installStore.Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("kmd has been deleted: %v", key)
			return nil
		}
		return err
	}

	kmd = kmd.DeepCopy()

	if !kmd.DeletionTimestamp.IsZero() {
		if !installv1alpha1.IsIntallModeFailed(kmd) && !installv1alpha1.IsConditionEmpty(kmd) {
			if kmd.GetLabels()[DisableCascadingDeletionLabel] == "false" {
				klog.InfoS("remove karmadaDeployment and karmada instance", "karmadaDeployment", kmd.Name)
				if err := c.factory.SyncWithAction(kmd, factory.UninstallAction); err != nil {
					return err
				}
			}
		}

		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			kmd, err := c.installStore.Get(name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}

			newer := kmd.DeepCopy()
			if !controllerutil.ContainsFinalizer(newer, ControllerFinalizer) {
				return nil
			}
			controllerutil.RemoveFinalizer(newer, ControllerFinalizer)

			if _, err := c.kmdClient.InstallV1alpha1().KarmadaDeployments().Update(context.TODO(), newer, metav1.UpdateOptions{}); err != nil {
				return err
			}
			return nil
		})
	}

	err = c.initDefaultValues(kmd)
	if err != nil {
		klog.Errorf("failed to init karmadaDeployment default value : %v", err)
		return err
	}
	return c.factory.Sync(kmd)
}

// initDefaultValues init karmadaDeployment default value
func (c *Controller) initDefaultValues(kmd *installv1alpha1.KarmadaDeployment) error {
	var err error
	var isUpdate bool
	// add default label karmadadeployments.install.karmada.io/disable-cascading-deletion:true
	if kmd.GetLabels() == nil {
		kmd.SetLabels(make(map[string]string))
	}
	kmdLabels := kmd.GetLabels()
	if _, isExist := kmdLabels[DisableCascadingDeletionLabel]; !isExist {
		kmdLabels[DisableCascadingDeletionLabel] = "false"
		kmd.SetLabels(kmdLabels)
		isUpdate = true
	}
	if len(kmd.Spec.ControlPlane.Namespace) == 0 {
		kmd.Spec.ControlPlane.Namespace = kmd.Name + "-" + rand.String(5)
	}
	if len(kmd.Spec.ControlPlane.ServiceType) == 0 {
		kmd.Spec.ControlPlane.ServiceType = corev1.ServiceTypeNodePort
	}

	// set karmada version by default
	if kmd.Spec.Images == nil {
		kmd.Spec.Images = &installv1alpha1.Images{
			KarmadaVersion: constants.DefaultKarmadaVersion,
		}
	} else if len(kmd.Spec.Images.KarmadaVersion) == 0 {
		kmd.Spec.Images.KarmadaVersion = constants.DefaultKarmadaVersion
	}

	// ensure finalizer
	if !controllerutil.ContainsFinalizer(kmd, ControllerFinalizer) && kmd.DeletionTimestamp.IsZero() {
		_ = controllerutil.AddFinalizer(kmd, ControllerFinalizer)
		isUpdate = true
	}
	if isUpdate {
		_, err = c.kmdClient.InstallV1alpha1().KarmadaDeployments().Update(context.TODO(), kmd, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
