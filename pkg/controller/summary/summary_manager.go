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
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/controller/summary/informermanager"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	installliter "github.com/daocloud/karmada-operator/pkg/generated/listers/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/status"
	clusterv1alpha1 "github.com/karmada-io/api/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/api/policy/v1alpha1"
)

var (
	// all gvr of resource that need be collected.
	serviceGvr           = corev1.SchemeGroupVersion.WithResource("services")
	secretGvr            = corev1.SchemeGroupVersion.WithResource("secrets")
	configmapGvr         = corev1.SchemeGroupVersion.WithResource("configmaps")
	pvcGvr               = corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims")
	deploymentGvr        = appsv1.SchemeGroupVersion.WithResource("deployments")
	overridePolicyGvr    = policyv1alpha1.SchemeGroupVersion.WithResource("overridepolicies")
	propagationpolicyGvr = policyv1alpha1.SchemeGroupVersion.WithResource("propagationpolicies")
	clusterGvr           = clusterv1alpha1.SchemeGroupVersion.WithResource("clusters")

	PolicyResources = []schema.GroupVersionResource{overridePolicyGvr, propagationpolicyGvr}
	CoreResources   = []schema.GroupVersionResource{serviceGvr, secretGvr, configmapGvr, pvcGvr}
)

type SummaryManager struct {
	kmdName   string
	kmdClient versioned.Interface

	installStore    installliter.KarmadaDeploymentLister
	informerManager informermanager.InformerManager

	closeOnce sync.Once
	close     chan struct{}
}

// NothingResourceEventHandler is a event handler that is notthing to do for informer.
type NothingResourceEventHandler struct{}

func (h *NothingResourceEventHandler) OnAdd(obj interface{})               {}
func (h *NothingResourceEventHandler) OnDelete(obj interface{})            {}
func (h *NothingResourceEventHandler) OnUpdate(oldObj, newObj interface{}) {}

func NewSummaryManager(kmdName string, kmdClient versioned.Interface,
	installStore installliter.KarmadaDeploymentLister, client dynamic.Interface) *SummaryManager {
	manager := &SummaryManager{
		kmdName:      kmdName,
		kmdClient:    kmdClient,
		installStore: installStore,
		close:        make(chan struct{}),
	}

	nothing := &NothingResourceEventHandler{}
	manager.informerManager = informermanager.NewInformerManager(client, 0, manager.close)

	// registry gvr of resource need be collect to dynamic informer. we only need to load thoes
	// resource to memory and not need tirgger any event.
	for _, gvr := range CoreResources {
		if !manager.informerManager.IsHandlerExist(gvr, nothing) {
			manager.informerManager.ForResource(gvr, nothing)
		}
	}
	for _, gvr := range PolicyResources {
		if !manager.informerManager.IsHandlerExist(gvr, nothing) {
			manager.informerManager.ForResource(gvr, nothing)
		}
	}
	if !manager.informerManager.IsHandlerExist(clusterGvr, nothing) {
		manager.informerManager.ForResource(clusterGvr, nothing)
	}
	if !manager.informerManager.IsHandlerExist(deploymentGvr, nothing) {
		manager.informerManager.ForResource(deploymentGvr, nothing)
	}

	return manager
}

func (m *SummaryManager) Run(shutdown <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.InfoS("Start summary manager")
	defer klog.InfoS("Shutting down summary manager")

	m.informerManager.Start()
	if !m.WaitForCacheSync() {
		return
	}

	go wait.Until(m.worker, time.Second, shutdown)

	go func() {
		<-shutdown
		m.Shuntdown()
	}()

	<-m.close
}

// worker is a loop to execute logic of collect resource. every time not to request
// karmada apiserver and list resource from informer lister.
func (m *SummaryManager) worker() {
	for m.processNext() {
		select {
		case <-m.close:
			return
		default:
			// TODO: sleep one secend.
			time.Sleep(time.Second)
		}
	}
}

func (m *SummaryManager) processNext() bool {
	kmd, err := m.installStore.Get(m.kmdName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("%v has been deleted", m.kmdName)
			return true
		}

		klog.ErrorS(err, "failed to get kmd", "kmd", m.kmdName)
		return true
	}

	// init a sumary if the status of kmd is nil.
	summary := &installv1alpha1.KarmadaResourceSummary{}
	if err := m.syncHandler(summary); err != nil {
		klog.ErrorS(err, "failed to sync kmd summary", "kmd", m.kmdName)
		return true
	}

	if err := m.updateKmdStatusSummaryIfNeed(kmd.DeepCopy(), summary); err != nil {
		klog.ErrorS(err, "failed to update kmd summary", "kmd", m.kmdName)
		return true
	}

	return true
}

// updateKmdStatusSummaryIfNeed to update summary of kmd if need. if the numerical value is same with
// current kmd summary. it will skip this updation.
func (m *SummaryManager) updateKmdStatusSummaryIfNeed(kmd *installv1alpha1.KarmadaDeployment, summary *installv1alpha1.KarmadaResourceSummary) error {
	if !equality.Semantic.DeepEqual(kmd.Status.Summary, summary) {
		klog.V(2).InfoS("Start to update kmd status summary", "kmd", kmd.Name)

		kmd.Status.Summary = summary
		if err := status.SetStatus(m.kmdClient, kmd); err != nil {
			klog.ErrorS(err, "Failed to update kmd status summary", "kmd", kmd.Name)
			return err
		}
	}

	return nil
}

func (m *SummaryManager) syncHandler(summary *installv1alpha1.KarmadaResourceSummary) error {
	if err := m.sumSyncHandler(summary); err != nil {
		return err
	}
	if err := m.clusterSyncHandler(summary); err != nil {
		return err
	}
	if err := m.workLoadsSyncHandler(summary); err != nil {
		return err
	}
	return nil
}

// workLoadsSyncHandler to calculate workload sum.
func (m *SummaryManager) workLoadsSyncHandler(summary *installv1alpha1.KarmadaResourceSummary) error {
	// TODO: only collect deployment sum, we should support more workloads,
	// e.g: sts, job, cornjob
	deployStore := m.informerManager.Lister(deploymentGvr)
	objs, err := deployStore.List(labels.Everything())
	if err != nil {
		return err
	}

	workLoadSummary := make(map[string]installv1alpha1.NumStatistic)

	var readyNum int32
	for _, obj := range objs {
		unstructured := obj.(*unstructured.Unstructured)

		var deploy appsv1.Deployment
		runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, &deploy)
		if checkDeploymentReady(deploy.Status.Conditions) {
			readyNum++
		}
	}

	totalNum := int32(len(objs))
	workLoadSummary[deploymentGvr.Resource] = installv1alpha1.NumStatistic{
		TotalNum: &totalNum,
		ReadyNum: &readyNum,
	}

	summary.WorkLoadSummary = workLoadSummary
	return nil
}

func (m *SummaryManager) clusterSyncHandler(summary *installv1alpha1.KarmadaResourceSummary) error {
	clusterStore := m.informerManager.Lister(clusterGvr)
	objs, err := clusterStore.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list cluster from karmada apiserver")
		return err
	}

	if len(objs) == 0 {
		return nil
	}

	nodeSummary := &installv1alpha1.NodeSummary{
		ClusterNodeSummary: make(map[string]installv1alpha1.NumStatistic),
	}
	resourceSummary := &installv1alpha1.ResourceSummary{
		ClusterResource: make(map[string]installv1alpha1.ResourceStatistic),
	}
	clusterSummary := &installv1alpha1.ClusterSummary{
		ClusterConditions: make(map[string][]metav1.Condition),
	}

	for _, obj := range objs {
		unstructured := obj.(*unstructured.Unstructured)

		var cluster clusterv1alpha1.Cluster
		runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, &cluster)

		csns := cluster.Status.NodeSummary
		if csns != nil {
			nodeSummary.ClusterNodeSummary[cluster.Name] = installv1alpha1.NumStatistic{
				TotalNum: &csns.TotalNum,
				ReadyNum: &csns.ReadyNum,
			}
		}

		csrs := cluster.Status.ResourceSummary
		if csrs != nil {
			resourceSummary.ClusterResource[cluster.Name] = installv1alpha1.ResourceStatistic{
				Allocatable: csrs.Allocatable,
				Allocating:  csrs.Allocating,
				Allocated:   csrs.Allocated,
			}
		}

		csct := cluster.Status.Conditions
		if csct != nil {
			clusterSummary.ClusterConditions[cluster.Name] = csct
		}
	}

	summary.NodeSummary = aggregateNodeSummary(nodeSummary)
	summary.ResourceSummary = aggregateResourceSummary(resourceSummary)
	summary.ClusterSummary = aggregateClusterSummary(clusterSummary)

	return nil
}

func (m *SummaryManager) sumSyncHandler(summary *installv1alpha1.KarmadaResourceSummary) error {
	if summary.ResourceTotalNum == nil {
		summary.ResourceTotalNum = make(map[string]int32, len(CoreResources))
	}
	if summary.PolicyTotalNum == nil {
		summary.PolicyTotalNum = make(map[string]int32, len(PolicyResources))
	}

	// calculate the total number of services.
	for _, gvr := range CoreResources {
		coreStore := m.informerManager.Lister(gvr)
		resources, err := coreStore.List(labels.Everything())
		if err != nil {
			return err
		}
		summary.ResourceTotalNum[gvr.Resource] = int32(len(resources))
	}

	// calculate the total number of services.
	for _, gvr := range PolicyResources {
		policyStore := m.informerManager.Lister(gvr)
		policies, err := policyStore.List(labels.Everything())
		if err != nil {
			return err
		}

		summary.PolicyTotalNum[gvr.Resource] = int32(len(policies))
	}
	return nil
}

func (m *SummaryManager) WaitForCacheSync() bool {
	cacheSync := m.informerManager.WaitForCacheSync()
	for _, isSync := range cacheSync {
		if !isSync {
			return false
		}
	}

	return true
}

func (m *SummaryManager) Shuntdown() {
	klog.InfoS("Shutting down kmd summary controller")
	m.closeOnce.Do(func() {
		close(m.close)
	})

	m.informerManager.Stop()
}

func aggregateNodeSummary(nodeSummary *installv1alpha1.NodeSummary) *installv1alpha1.NodeSummary {
	var readyNum, totalNum int32
	for _, ns := range nodeSummary.ClusterNodeSummary {
		readyNum += *ns.ReadyNum
		totalNum += *ns.TotalNum
	}

	nodeSummary.ReadyNum = &readyNum
	nodeSummary.TotalNum = &totalNum
	return nodeSummary
}

func aggregateResourceSummary(resourceSummary *installv1alpha1.ResourceSummary) *installv1alpha1.ResourceSummary {
	for _, cr := range resourceSummary.ClusterResource {
		resourceSummary.Allocatable = calculateResourceList(resourceSummary.Allocatable, cr.Allocatable)
		resourceSummary.Allocated = calculateResourceList(resourceSummary.Allocated, cr.Allocated)
		resourceSummary.Allocating = calculateResourceList(resourceSummary.Allocating, cr.Allocating)
	}

	return resourceSummary
}

func calculateResourceList(totalResource, summateResource corev1.ResourceList) corev1.ResourceList {
	if totalResource == nil {
		totalResource = make(map[corev1.ResourceName]resource.Quantity)
	}
	cpu := totalResource.Cpu()
	cpu.Add(*summateResource.Cpu())
	totalResource[corev1.ResourceCPU] = *cpu

	memory := totalResource.Memory()
	memory.Add(*summateResource.Memory())
	totalResource[corev1.ResourceMemory] = *memory

	storage := totalResource.Storage()
	storage.Add(*summateResource.Storage())
	totalResource[corev1.ResourceStorage] = *storage

	storageEphemeral := totalResource.StorageEphemeral()
	storageEphemeral.Add(*summateResource.StorageEphemeral())
	totalResource[corev1.ResourceEphemeralStorage] = *storageEphemeral

	pods := totalResource.Pods()
	pods.Add(*summateResource.Pods())
	totalResource[corev1.ResourcePods] = *pods

	return totalResource
}

func aggregateClusterSummary(clusterSummary *installv1alpha1.ClusterSummary) *installv1alpha1.ClusterSummary {
	var readyNum int32
	for _, cs := range clusterSummary.ClusterConditions {
		if checkClusterReady(cs) {
			readyNum++
		}
	}

	totalNum := int32(len(clusterSummary.ClusterConditions))
	clusterSummary.ReadyNum = &readyNum
	clusterSummary.TotalNum = &totalNum
	return clusterSummary
}

func checkClusterReady(conditions []metav1.Condition) bool {
	for _, condtion := range conditions {
		if condtion.Type == clusterv1alpha1.ClusterConditionReady && condtion.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

func checkDeploymentReady(conditions []appsv1.DeploymentCondition) bool {
	for _, condtion := range conditions {
		if condtion.Type == appsv1.DeploymentAvailable && condtion.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
