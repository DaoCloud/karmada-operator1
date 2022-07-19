package controller

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	clientset "github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	installinformers "github.com/daocloud/karmada-operator/pkg/generated/informers/externalversions/install/v1alpha1"
	installliter "github.com/daocloud/karmada-operator/pkg/generated/listers/install/v1alpha1"
)

const (
	maxClusterSynchroRetry = 15
	ControllerFinalizer    = "karmada.install.io/installer-controller"
)

type Controller struct {
	runLock sync.Mutex
	stopCh  <-chan struct{}

	client        clientset.Interface
	queue         workqueue.RateLimitingInterface
	installLister installliter.KarmadaDeploymentLister
}

func NewController(client clientset.Interface, karmadaDeploymentInformer installinformers.KarmadaDeploymentInformer) *Controller {
	controller := &Controller{
		client:        client,
		installLister: karmadaDeploymentInformer.Lister(),
		queue: workqueue.NewRateLimitingQueue(
			workqueue.NewItemExponentialFailureRateLimiter(2*time.Second, 5*time.Second),
		),
	}

	karmadaDeploymentInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.enqueue,
			DeleteFunc: controller.enqueue,

			UpdateFunc: func(older, newer interface{}) {
				oldObj := older.(*installv1alpha1.KarmadaDeployment)
				newObj := newer.(*installv1alpha1.KarmadaDeployment)
				if newObj.DeletionTimestamp.IsZero() && equality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) {
					return
				}
				controller.enqueue(newer)
			},
		},
	)
	return controller
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	c.queue.Add(key)
}

func (c *Controller) Run(workers int, stopCh <-chan struct{}) {
	c.runLock.Lock()
	defer c.runLock.Unlock()

	if c.stopCh != nil {
		return
	}

	klog.InfoS("Karmada Operator is running", "workers", workers)

	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			c.worker()
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

func (c *Controller) processNext() (continued bool) {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)
	continued = true

	// KarmadaDeployment is cluster scope, key == name
	_, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		klog.ErrorS(err, "failed to split karmadaDeployment key", "key", key)
		return
	}

	kd, err := c.installLister.Get(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get karmadaDeployment from lister", "policy", name)
			return
		}
		return
	}
	if err := c.reconcile(kd); err != nil {
		klog.ErrorS(err, "Failed to reconcile karmadaDeployment", "cluster", name, "num requeues", c.queue.NumRequeues(key))
		if c.queue.NumRequeues(key) < maxClusterSynchroRetry {
			c.queue.AddRateLimited(key)
			return
		}
		klog.V(2).Infof("Dropping karmadaDeployment %q out of the queue: %v", key, err)
	}

	c.queue.Forget(key)
	return
}

func (c *Controller) reconcile(kd *installv1alpha1.KarmadaDeployment) (err error) {
	if !kd.DeletionTimestamp.IsZero() {
		klog.InfoS("remove karmadaDeployment", "karmadaDeployment", kd.Name)

		// TODO: delete event

		if !controllerutil.ContainsFinalizer(kd, ControllerFinalizer) {
			return nil
		}

		if removed := controllerutil.RemoveFinalizer(kd, ControllerFinalizer); removed {

			_, err := c.client.InstallV1alpha1().KarmadaDeployments().
				Update(context.TODO(), kd, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
		return nil
	}

	// ensure finalizer
	if !controllerutil.ContainsFinalizer(kd, ControllerFinalizer) {
		if updated := controllerutil.AddFinalizer(kd, ControllerFinalizer); updated {

			_, err := c.client.InstallV1alpha1().KarmadaDeployments().
				Update(context.TODO(), kd, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return err
}
