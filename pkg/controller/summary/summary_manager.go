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
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/controller/summary/informermanager"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	installliter "github.com/daocloud/karmada-operator/pkg/generated/listers/install/v1alpha1"
	clusterv1alpha1 "github.com/karmada-io/api/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/api/policy/v1alpha1"
)

var (
	serviceGvr   = corev1.SchemeGroupVersion.WithResource("services")
	SecretGvr    = corev1.SchemeGroupVersion.WithResource("secrets")
	configmapGvr = corev1.SchemeGroupVersion.WithResource("configmaps")
	pvcGvr       = corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims")

	// serviceGvr   = schema.GroupVersionResource{Group: "core", Version: "v1", Resource: "services"}
	// SecretGvr    = schema.GroupVersionResource{Group: "core", Version: "v1", Resource: "secrets"}
	// configmapGvr = schema.GroupVersionResource{Group: "core", Version: "v1", Resource: "configmaps"}
	// pvcGvr       = schema.GroupVersionResource{Group: "core", Version: "v1", Resource: "persistentvolumeclaims"}

	deploymentGvr        = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	overridePolicyGvr    = policyv1alpha1.SchemeGroupVersion.WithResource("overridepolicies")
	propagationpolicyGvr = policyv1alpha1.SchemeGroupVersion.WithResource("propagationpolicies")

	ClusterGvr = clusterv1alpha1.SchemeGroupVersion.WithResource("clusters")

	PolicyResources = []schema.GroupVersionResource{overridePolicyGvr, propagationpolicyGvr}

	// TODO: corev1 resource
	CoreResources = []schema.GroupVersionResource{}
)

type SummaryManager struct {
	kmdName   string
	kmdClient versioned.Interface

	installStore    installliter.KarmadaDeploymentLister
	informerManager informermanager.InformerManager

	closeOnce sync.Once
	close     chan struct{}
}

func NewSummaryManager(kmdName string, kmdClient versioned.Interface,
	installStore installliter.KarmadaDeploymentLister, client dynamic.Interface) *SummaryManager {
	manager := &SummaryManager{
		kmdName:      kmdName,
		kmdClient:    kmdClient,
		installStore: installStore,
		close:        make(chan struct{}),
	}

	manager.informerManager = informermanager.NewInformerManager(client, 0, manager.close)
	for _, gvr := range CoreResources {
		if !manager.informerManager.IsHandlerExist(gvr, nil) {
			manager.informerManager.ForResource(gvr, nil)
		}
	}
	for _, gvr := range PolicyResources {
		if !manager.informerManager.IsHandlerExist(gvr, nil) {
			manager.informerManager.ForResource(gvr, nil)
		}
	}
	if !manager.informerManager.IsHandlerExist(ClusterGvr, nil) {
		manager.informerManager.ForResource(ClusterGvr, nil)
	}
	if !manager.informerManager.IsHandlerExist(ClusterGvr, nil) {
		manager.informerManager.ForResource(deploymentGvr, nil)
	}

	return manager
}

func (m *SummaryManager) Run(shutdown <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Start summary manager")
	defer klog.Infof("Shutting down summary manager")

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

func (m *SummaryManager) worker() {
	for {
		_, err := m.processNextWorkItem()
		if err != nil {
			klog.Errorf("failed sync karmadadeployment summary, err: %v", err)
		}
	}
}

func (m *SummaryManager) processNextWorkItem() (bool, error) {
	kmd, err := m.installStore.Get(m.kmdName)
	if err != nil {
		return false, err
	}

	summary := kmd.Status.Summary
	if summary == nil {
		summary = &installv1alpha1.KarmadaResourceSummary{
			ResourceTotalNum: make(map[string]int32),
			PolicyTotalNum:   make(map[string]int32),
		}
	}

	if err := m.syncHandler(summary); err != nil {
		return false, err
	}

	if err := m.updateKmdStatusSummaryIfNeed(kmd.DeepCopy(), summary); err != nil {
		return false, err
	}

	return true, nil
}

func (m *SummaryManager) updateKmdStatusSummaryIfNeed(kmd *installv1alpha1.KarmadaDeployment, summary *installv1alpha1.KarmadaResourceSummary) error {
	if !equality.Semantic.DeepEqual(kmd.Status.Summary, summary) {
		klog.V(2).Infof("Start to update kmd %s status summary", kmd.Name)

		kmd.Status.Summary = summary
		_, err := m.kmdClient.InstallV1alpha1().KarmadaDeployments().UpdateStatus(context.TODO(), kmd, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed to update kmd %s status summary, err:", kmd.Name, err)
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
		deploy := obj.(*appsv1.Deployment)
		if checkDeploymentReady(deploy.Status.Conditions) {
			readyNum++
		}
	}
	workLoadSummary[deploymentGvr.Resource] = installv1alpha1.NumStatistic{
		TotalNum: int32(len(objs)),
		ReadyNum: readyNum,
	}

	summary.WorkLoadSummary = workLoadSummary
	return nil
}

func (m *SummaryManager) clusterSyncHandler(summary *installv1alpha1.KarmadaResourceSummary) error {
	clusterStore := m.informerManager.Lister(ClusterGvr)
	objs, err := clusterStore.List(labels.Everything())
	if err != nil {
		return err
	}

	if len(objs) == 0 {
		summary.ClusterSummary = &installv1alpha1.ClusterSummary{}
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
		cluster := obj.(*clusterv1alpha1.Cluster)

		csns := cluster.Status.NodeSummary
		if csns != nil {
			nodeSummary.ClusterNodeSummary[cluster.Name] = installv1alpha1.NumStatistic{
				TotalNum: csns.TotalNum,
				ReadyNum: csns.ReadyNum,
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
	klog.Infof("Shutting down kmd summary controller")
	m.closeOnce.Do(func() {
		close(m.close)
	})

	m.informerManager.Stop()
}

func aggregateNodeSummary(nodeSummary *installv1alpha1.NodeSummary) *installv1alpha1.NodeSummary {
	for _, ns := range nodeSummary.ClusterNodeSummary {
		nodeSummary.ReadyNum += ns.ReadyNum
		nodeSummary.TotalNum += ns.TotalNum
	}

	return nodeSummary
}

func aggregateResourceSummary(resourceSummary *installv1alpha1.ResourceSummary) *installv1alpha1.ResourceSummary {
	for _, cr := range resourceSummary.ClusterResource {
		calculateResourceList(resourceSummary.Allocatable, cr.Allocatable)
		calculateResourceList(resourceSummary.Allocated, cr.Allocated)
		calculateResourceList(resourceSummary.Allocating, cr.Allocating)
	}

	return resourceSummary
}

func calculateResourceList(totalResource, summateResource corev1.ResourceList) {
	totalResource.Cpu().Add(*summateResource.Cpu())
	totalResource.Memory().Add(*summateResource.Memory())
	totalResource.Storage().Add(*summateResource.Storage())
	totalResource.StorageEphemeral().Add(*summateResource.StorageEphemeral())
	totalResource.Pods().Add(*summateResource.Pods())
}

func aggregateClusterSummary(clusterSummary *installv1alpha1.ClusterSummary) *installv1alpha1.ClusterSummary {
	var readyNum int32
	for _, cs := range clusterSummary.ClusterConditions {
		if checkClusterReady(cs) {
			readyNum++
		}
	}

	clusterSummary.ReadyNum = readyNum
	clusterSummary.TotalNum = int32(len(clusterSummary.ClusterConditions))
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
