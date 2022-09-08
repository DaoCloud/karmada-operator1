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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TOOD: kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.conditions[?(@.type == 'ControlPlaneReady')].reason",description=""

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:scope="Cluster",path=karmadadeployments,shortName=kmd;kmds
// +kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=".spec.mode",description=""
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase",description=""
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.controlPlaneReady",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// KarmadaDeployment enables declarative installation of karmada.
type KarmadaDeployment struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired behavior of the KarmadaDeployment.
	// +optional
	Spec KarmadaDeploymentSpec `json:"spec,omitempty"`

	// Most recently observed status of the KarmadaDeployment.
	// +optional
	Status KarmadaDeploymentStatus `json:"status,omitempty"`
}

// KarmadaDeploymentSpec is the specification of the desired behavior of the KarmadaDeployment.
type KarmadaDeploymentSpec struct {
	// mode of karmada installtion, can be "Helm" or "Karmadactl",
	// Default is Helm.
	// +optional
	// +kubebuilder:validation:Enum=Helm;Karmadactl
	Mode *InstallMode `json:"mode,omitempty"`

	// set global images registry and version, kubenetes original images and
	// karmada images can be set up separately. If there is nil, will use the
	// default images to install.
	// +optional
	Images *Images `json:"images,omitempty"`

	// All of config of karmada control plane installtion.
	ControlPlane ControlPlaneCfg `json:"controlPlane,omitempty"`
}

// +enum
type InstallMode string

const (
	// Use charts provided by karmada community.
	HelmMode InstallMode = "Helm"
	// Use karmadactl command-line tools to install karmada.
	KarmadactlMode InstallMode = "Karmadactl"
)

// karmada components, these are optional components for karmada and
// they does not affect the normal feature of karmada.
// +enum
type Component string

const (
	SchedulerEstimatorComponent Component = "schedulerEstimator"
	DeschedulerComponent        Component = "descheduler"
	SearchComponent             Component = "search"
)

// karmada modules that must be install by karmada, they will affect
// the normal feature of karmada. but the etcd can be supported by extenal.
// +enum
type ModuleName string

const (
	SchedulerModuleName             ModuleName = "scheduler"
	WebhookModuleName               ModuleName = "webhook"
	ControllerManagerModuleName     ModuleName = "controllerManager"
	AgentModuleName                 ModuleName = "agent"
	AggregatedApiserverModuleName   ModuleName = "aggregatedApiServer"
	EtcdModuleName                  ModuleName = "etcd"
	KubeApiserverModuleName         ModuleName = "apiServer"
	KubeControllerManagerModuleName ModuleName = "kubeControllerManager"
	SchedulerEstimatorModuleName    ModuleName = "schedulerEstimator"
	DeschedulerModuleName           ModuleName = "descheduler"
	SearchModuleName                ModuleName = "search"
)

type ControlPlaneCfg struct {
	// specified namespace to karmada, if not specified defualt
	// "karmada-system".
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// the install cluster access credentials,if not specified,
	// it will be installed in the cluster where the operator is located by default.
	// There are two way to connect cluster, one is by kubeconfig file path. two is restore
	// the credentials in the secret.
	EndPointCfg *EndPointCfg `json:"endPointCfg,omitempty"`

	// For karmada, two types etcd can be used:
	// Internal etcd: must be installed by operator.
	//    storageMode: epmtydir, hostPath, pvc
	// External etcd: not need be installed by operator.
	// +optional
	ETCD *ETCD `json:"etcd,omitempty"`

	// set karmada modules images and replicas separately.
	// the module name must be enum.
	// +optional
	Modules []Module `json:"modules,omitempty"`

	// optional karmada components, no components be installed
	// by defualt. component value must be enum.
	// +optional
	Components []Component `json:"components,omitempty"`

	// default is true, if not installed karmada crd resources
	// in cluster, please set it to false.
	// +optional
	InstallCRD *bool `json:"installCRD,omitempty"`

	// ServiceType represents the sevice type of karmada apiserver.
	// it is Nodeport by default.
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// support extar chart values to a configmap, the chart values
	// format exactly reference to origin chart value.yaml.
	// +optional
	ChartExtraValues *LocalConfigmapReference `json:"chartExtraValues,omitempty"`
}

type Module struct {
	// karmada module name, the name must be
	// +optional
	// +kubebuilder:validation:Enum=scheduler;webhook;controllerManager;agent;aggregatedApiServer;etcd;apiServer;kubeControllerManager;schedulerEstimator;descheduler;search
	Name ModuleName `json:"name,omitempty"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Image string `json:"image,omitempty"`
}

type Images struct {
	// karmada regisrty, default is "swr.ap-southeast-1.myhuaweicloud.com".
	// all of karmada images is effect by the param.
	// +optional
	KarmadaRegistry string `json:"karmadaRegistry,omitempty"`

	// karmada images version, default is "latest".
	// all of karmada images is effect by the param.
	// +optional
	KarmadaVersion string `json:"karmadaVersion,omitempty"`

	// the registry of kube-apiserver and kube-controller-manager, default is "k8s.gcr.io".
	// +optional
	KubeRegistry string `json:"kubeRegistry,omitempty"`

	// the iamge version of kube-apiserver and kube-controller-manager.
	// +optional
	KubeVersion string `json:"kubeVersion,omitempty"`

	// the registry of cfssl and kubectl, default is "docker.io".
	// +optional
	DockerIoRegistry string `json:"dockerIoRegistry,omitempty"`
}

type ETCD struct {
	// TODO: support external etcd.
	// StorageMode etcd data storage mode(emptyDir,hostPath, PVC), default "emptyDir"
	// +optional
	StorageMode string `json:"storageMode,omitempty"`

	// If specified StorageMode is PVC, StorageClass must be set.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// If specified StorageMode is PVC, Size must be set.
	// +optional
	Size string `json:"size,omitempty"`
}

type EndPointCfg struct {
	// +optional
	Kubeconfig string `json:"kubeconfig,omitempty"`

	// The API endpoint of the member cluster. This can be a hostname,
	// hostname:port, IP or IP:port.
	// +optional
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint,omitempty"`

	// SecretRef represents the secret contains mandatory credentials to access the member cluster.
	// The secret should hold credentials as follows:
	// - secret.data.token
	// - secret.data.caBundle
	// +optional
	SecretRef *LocalSecretReference `json:"secretRef,omitempty"`
}

// the whole installtion process is be divide diffrent phase.
//     Preflight: preparatory work, build cluster clientset pull karmada chart pkg.
//     Installing: calculate chart values, install karmada chart by helm client.
//     Waiting: wait to all of karmada pods to be ready.
//     ControlPlaneReady: check karmada cluster is healthy.
//     DeployFailedPhase: the karmada cluster is not healthy.
// +enum
type Phase string

const (
	PreflightPhase         Phase = "Preflight"
	DeployingPhase         Phase = "Deploying"
	WaitingPhase           Phase = "Waiting"
	InstalledCRDPhase      Phase = "InstalledCRD"
	ControlPlaneReadyPhase Phase = "Completed"
)

type KarmadaDeploymentStatus struct {
	// ObservedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// the karmada installtion phase, must be enum type.
	// +required
	Phase Phase `json:"phase,omitempty"`

	// ControlPlaneReady represent karmada cluster is health status.
	// +required
	ControlPlaneReady bool `json:"controlPlaneReady,omitempty"`

	// after the karmada installed, restore the kubeconfig to secret.
	SecretRef *LocalSecretReference `json:"secretRef,omitempty"`

	// KarmadaVersion represente the karmada verison.
	KarmadaVersion string `json:"karmadaVersion,omitempty"`

	// KubernetesVersion represente the karmada-apiserver version.
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// MemberClusterSummary gather all of member cluster summary information.
	Summary *KarmadaResourceSummary `json:"summary,omitempty"`

	// Conditions represents the latest available observations of a karmadaDeployment's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// KarmadaResourceSummary specifies some resource totals for the karmada control plane
// and some resource totals for the member cluster that we want to collect.
type KarmadaResourceSummary struct {
	// ResourceTotalNum represent the total number of distributed resource on karmada control plane.
	// e.g: service, ingress, pvc, secret.
	// +optional
	ResourceTotalNum map[string]int32 `json:"resourceTotalNum,omitempty"`

	// PolicyTotalNum represent the total number of policy resource.
	// e.g: PropagationPolicy, OverridePolicy.
	// +optional
	PolicyTotalNum map[string]int32 `json:"policyTotalNum,omitempty"`

	// ClusterSummary represents the each member cluster summary.
	// +optional
	ClusterSummary *ClusterSummary `json:"clusterSummary,omitempty"`

	// NodeSummary represents all nodes summary of all clusters.
	// +optional
	NodeSummary *NodeSummary `json:"nodeSummary,omitempty"`

	// ResourceSummary represents the summary of resources in all of the member cluster.
	// +optional
	ResourceSummary *ResourceSummary `json:"resourceSummary,omitempty"`

	// WorkLoadSummary represents all workLoads summary of all clusters.
	// e.g: deployment, statefulset.
	// +optional
	WorkLoadSummary map[string]NumStatistic `json:"workLoadSummary,omitempty"`
}

type NodeSummary struct {
	NumStatistic `json:",inline"`

	// ClusterNodeSummary is node summary of each cluster.
	// +optional
	ClusterNodeSummary map[string]NumStatistic `json:"clusterNodeSummary,omitempty"`
}

// ClusterSummary specifies all member cluster state and each member cluster conditions.
type ClusterSummary struct {
	NumStatistic `json:",inline"`

	// ClusterConditions is conditions of each cluster.
	// +optional
	ClusterConditions map[string][]metav1.Condition `json:"clusterConditions,omitempty"`
}

// ClusterSummary specifies all member cluster state and conditions.
type ResourceSummary struct {
	ResourceStatistic `json:",inline"`

	// ClusterResource represents the resources of each cluster.
	// +optional
	ClusterResource map[string]ResourceStatistic `json:"clusterResource,omitempty"`
}

type ResourceStatistic struct {
	// Allocatable represents the resources of all clusters that are available for scheduling.
	// Total amount of allocatable resources on all nodes of all clusters.
	// +optional
	Allocatable corev1.ResourceList `json:"allocatable,omitempty"`

	// Allocating represents the resources of all clusters that are pending for scheduling.
	// Total amount of required resources of all Pods of all clusters that are waiting for scheduling.
	// +optional
	Allocating corev1.ResourceList `json:"allocating,omitempty"`

	// Allocated represents the resources of all clusters that have been scheduled.
	// Total amount of required resources of all Pods of all clusters that have been scheduled to nodes.
	// +optional
	Allocated corev1.ResourceList `json:"allocated,omitempty"`
}

type NumStatistic struct {
	// TotalNum represents the total number of resource of member cluster.
	// +optional
	TotalNum *int32 `json:"totalNum,omitempty"`

	// ReadyNum represents the resource in ready state.
	// +optional
	ReadyNum *int32 `json:"readyNum,omitempty"`
}

// WorkloadSummary represents the summary of workload state in a specific cluster.
type WorkloadSummary struct {
	// Statistic represent  workload total number and ready member of all of member cluster.
	NumStatistic

	// ClusterWorkload represent cluster
	// +optional
	ClusterWorkload map[string]NumStatistic `json:"clusterWorkload,omitempty"`
}

// LocalSecretReference is a reference to a secret within the enclosing
// namespace.
type LocalSecretReference struct {
	// Namespace is the namespace for the resource being referenced.
	Namespace string `json:"namespace,omitempty"`

	// Name is the name of resource being referenced.
	Name string `json:"name,omitempty"`
}

// LocalConfigmapReference is a reference to a configmap within the enclosing
// namespace.
type LocalConfigmapReference struct {
	// Namespace is the namespace for the resource being referenced.
	Namespace string `json:"namespace,omitempty"`

	// Name is the name of resource being referenced.
	Name string `json:"name,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type KarmadaDeploymentList struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KarmadaDeployment `json:"items"`
}
