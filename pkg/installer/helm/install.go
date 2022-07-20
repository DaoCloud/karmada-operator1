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

package helm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	"github.com/daocloud/karmada-operator/pkg/status"
)

const (
	// TODO:
	chartBasePath    = "/var/run/karmada-operator"
	KarmadaNamespace = "karmada-system"
	DefaulTimeout    = time.Second * 120
	WaitPodTimeout   = time.Second * 60
)

var (
	Kubernates = []string{"apiServer", "kubeControllerManager"}
	Karmada    = []string{"schedulerEstimator", "descheduler", "search", "scheduler", "webhook", "controllerManager", "agent", "aggregatedApiServer"}
)

type installWorkflow struct {
	release   *helm.Release
	values    []byte
	chartPath string
	destHost  string

	client        clientset.Interface
	helmClient    helm.Client
	kmdClient     versioned.Interface
	destClient    clientset.Interface
	chartResource *ChartResource
}

func NewInstallWorkflow(destHost string,
	helmClient helm.Client,
	kmdClient versioned.Interface,
	destClient clientset.Interface,
	client clientset.Interface,
	chartResource *ChartResource) *installWorkflow {

	return &installWorkflow{
		destHost:      destHost,
		kmdClient:     kmdClient,
		helmClient:    helmClient,
		destClient:    destClient,
		client:        client,
		chartResource: chartResource,
	}
}

func (install *installWorkflow) Install(kmd *installv1alpha1.KarmadaDeployment) error {
	var err error
	switch kmd.Status.Phase {
	case "", installv1alpha1.PreflightPhase:
		if err = install.Preflight(kmd); err != nil {
			return err
		}
		if err := install.Deploy(kmd); err != nil {
			return err
		}

		fallthrough
	case installv1alpha1.DeployedPhase, installv1alpha1.WaitingPhase:
		if err := install.Wait(kmd); err != nil {
			return err
		}
		if err := install.Completed(kmd); err != nil {
			return err
		}
	case installv1alpha1.ControlPlaneReadyPhase:
		return nil
	}
	return nil
}

func (install *installWorkflow) Preflight(kmd *installv1alpha1.KarmadaDeployment) error {
	var err error
	if err = status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.PreflightPhase); err != nil {
		return err
	}

	install.chartPath, _, err = FetchChart(install.helmClient, install.chartResource)
	if err != nil {
		return err
	}

	if install.values == nil {
		if install.values, err = ComposeValues(kmd); err != nil {
			return err
		}
	}

	return nil
}

func FetchChart(helmClient helm.Client, source *ChartResource) (string, bool, error) {
	repoPath, filename, err := makeChartPath(chartBasePath, source)
	if err != nil {
		return "", false, err
	}
	chartPath := filepath.Join(repoPath, filename)
	stat, err := os.Stat(chartPath)
	switch {
	case os.IsNotExist(err):
		chartPath, err = helmClient.PullWithRepoURL(source.RepoURL, source.Name, source.Version, repoPath)
		if err != nil {
			return chartPath, false, err
		}
		return chartPath, true, nil
	case err != nil:
		return chartPath, false, err
	case stat.IsDir():
		return chartPath, false, errors.New("path to chart exists but is a directory")
	}
	return chartPath, false, nil
}

func makeChartPath(base string, source *ChartResource) (string, string, error) {
	repoPath := filepath.Join(base, source.CleanRepoURL())
	if err := os.MkdirAll(repoPath, 00750); err != nil {
		return "", "", err
	}
	return repoPath, fmt.Sprintf("%s-%s.tgz", source.Name, source.Version), nil
}

// CleanRepoURL returns the RepoURL but removes the query string and ensures
// it ends with a trailing slash.
func (c ChartResource) CleanRepoURL() string {
	cleanURL, err := url.Parse(c.RepoURL)
	if err != nil {
		return strings.TrimSuffix(c.RepoURL, "/") + "/"
	}
	cleanURL.Path = strings.TrimSuffix(cleanURL.Path, "/") + "/"
	cleanURL.RawQuery = ""
	cleanURL.Scheme = ""
	return cleanURL.String()
}

func ComposeValues(kd *installv1alpha1.KarmadaDeployment) ([]byte, error) {
	values := make(helm.Values)
	controlPlane := kd.Spec.ControlPlane

	// Parse etcd values
	if controlPlane.ETCD != nil {
		var (
			etcdStorageModeKey  = "etcd.internal.storageType"
			etcdStorageSize     = "etcd.internal.pvc.size"
			etcdStorageClassKey = "etcd.internal.pvc.storageClass"
		)
		if len(controlPlane.ETCD.StorageMode) > 0 {
			values[etcdStorageModeKey] = controlPlane.ETCD.StorageMode
		}
		if len(controlPlane.ETCD.StorageClass) > 0 {
			values[etcdStorageClassKey] = controlPlane.ETCD.StorageClass
		}
		if len(controlPlane.ETCD.Size) > 0 {
			values[etcdStorageSize] = controlPlane.ETCD.Size
		}
	}

	// Parse module image and replicas values.
	for _, module := range controlPlane.Modules {
		name := string(module.Name)
		var (
			registry, repository, tag string

			registryKey      = fmt.Sprintf("%s.image.registry", name)
			repositoryKey    = fmt.Sprintf("%s.image.repository", name)
			tagKey           = fmt.Sprintf("%s.image.tag", name)
			replicasCountKey = fmt.Sprintf("%s.replicaCount", name)
		)

		if module.Replicas != nil {
			values[replicasCountKey] = module.Replicas
		}
		if len(module.Image) > 0 {
			image := module.Image
			i := strings.LastIndex(image, ":")
			if i > 0 {
				tag = image[i+1:]
				values[tagKey] = tag
				image = image[:i]
			}
			if strings.Contains(image, "/") {
				registry, repository, _ = strings.Cut(image, "/")
			} else {
				repository = image
			}
			values[registryKey] = registry
			values[repositoryKey] = repository
		}
	}

	// Parse global images.
	if kd.Spec.Images != nil {
		for _, k := range Karmada {
			if len(kd.Spec.Images.KarmadaRegistry) > 0 {
				values[fmt.Sprintf("%s.image.registry", k)] = kd.Spec.Images.KarmadaRegistry
			}
			if len(kd.Spec.Images.KarmadaVersion) > 0 {
				values[fmt.Sprintf("%s.image.tag", k)] = kd.Spec.Images.KarmadaVersion
			}
		}
		for _, k := range Kubernates {
			if len(kd.Spec.Images.KubeResgistry) > 0 {
				values[fmt.Sprintf("%s.image.registry", k)] = kd.Spec.Images.KubeResgistry
			}
			if len(kd.Spec.Images.KubeVersion) > 0 {
				values[fmt.Sprintf("%s.image.tag", k)] = kd.Spec.Images.KubeVersion
			}
		}
	}

	if len(controlPlane.Components) > 0 {
		values["components"] = controlPlane.Components
	}

	// TODO: load chart config of configmap
	return values.YAML()
}

func (install *installWorkflow) Deploy(kmd *installv1alpha1.KarmadaDeployment) error {
	if len(kmd.Spec.ControlPlane.Namespace) == 0 {
		kmd.Spec.ControlPlane.Namespace = KarmadaNamespace
	}

	ns, err := install.destClient.CoreV1().Namespaces().Get(context.TODO(), kmd.Spec.ControlPlane.Namespace, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get namespace from dest cluster", "namespace", kmd.Spec.ControlPlane.Namespace)
			return err
		}

		if ns, err = install.destClient.CoreV1().Namespaces().Create(context.TODO(),
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kmd.Spec.ControlPlane.Namespace}}, metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	releaseName := fmt.Sprintf("karmada-%s", kmd.Name)
	install.release, err = install.helmClient.UpgradeFromPath(install.chartPath, releaseName, install.values, helm.UpgradeOptions{
		Namespace:         ns.Name,
		Timeout:           DefaulTimeout,
		Install:           true,
		Force:             true,
		SkipCRDs:          false,
		Wait:              false,
		Atomic:            true,
		DisableValidation: false,
	})
	if err != nil {
		return err
	}

	return status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.DeployedPhase)
}

func (install *installWorkflow) Wait(kmd *installv1alpha1.KarmadaDeployment) error {
	release, err := install.GetRelease(kmd)
	if err != nil {
		return err
	}

	kmd.Status.Phase = installv1alpha1.WaitingPhase
	if err := status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.WaitingPhase); err != nil {
		return err
	}

	karmadaApiserver := fmt.Sprintf("%s-karmada-apiserver", release.Name)
	for _, m := range []string{karmadaApiserver, "etcd"} {
		if _, err := waitForPodReady(install.client, release.Namespace, m); err != nil {
			return err
		}
	}

	return nil
}

func waitForPodReady(client clientset.Interface, namespace, deploymentName string) (*corev1.Pod, error) {
	ctx, cancelFun := context.WithTimeout(context.TODO(), WaitPodTimeout)
	defer cancelFun()

	label := labels.Set{"app": deploymentName}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: label.String(),
			})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
				LabelSelector: label.String(),
			})
		},
	}

	// TODO:
	preconditionFunc := func(store cache.Store) (bool, error) { return true, nil }

	conditionFunc := func(event watch.Event) (bool, error) {
		if pod, ok := event.Object.(*corev1.Pod); ok {
			for _, c := range pod.Status.Conditions {
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
		return false, fmt.Errorf("event object not of type Pod")
	}

	event, err := toolswatch.UntilWithSync(ctx, lw, &corev1.Pod{}, preconditionFunc, conditionFunc)
	if err != nil {
		return nil, fmt.Errorf("timeout waiting for mudile %s to ready: %v", deploymentName, err)
	}

	if event == nil {
		return nil, nil
	}

	if pod, ok := event.Object.(*corev1.Pod); ok {
		return pod, nil
	}
	return nil, fmt.Errorf("event object not of type Pod")
}

func (install *installWorkflow) Completed(kmd *installv1alpha1.KarmadaDeployment) error {
	release, err := install.GetRelease(kmd)
	if err != nil {
		return err
	}
	secretmapName := fmt.Sprintf("%s-kubeconfig", release.Name)
	secret, err := install.destClient.CoreV1().Secrets(release.Namespace).Get(context.TODO(), secretmapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// TODO: expose the karmada service
	// client, err := BuildClientsetFormKubeconfig(secret.Data["kubeconfig"])
	// if err != nil {
	// 	return err
	// }

	// version, err := client.ServerVersion()
	// if err != nil {
	// 	return err
	// }
	// klog.V(2).Info("Success install karmada release, version:", version.String())

	// restore the karmda kubeconfig to secret, and set the secret info to kmd status.
	secretCopy := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: kmd.Namespace,
		},
		Data: secret.Data,
		Type: secret.Type,
	}
	if secretCopy, err = install.client.CoreV1().Secrets(kmd.Namespace).Create(context.TODO(), secretCopy, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {

			secretCopy, err = install.client.CoreV1().Secrets(kmd.Namespace).Update(context.TODO(), secretCopy, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	// The flow is completed, set the controlPlaneReady.
	kmd.Status.SecretRef = &installv1alpha1.LocalSecretReference{
		Name:      secretCopy.Name,
		Namespace: secretCopy.Namespace,
	}

	kmd.Status.ControlPlaneReady = true
	return status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.ControlPlaneReadyPhase)
}

func BuildClientsetFormKubeconfig(kubeconfig []byte) (*clientset.Clientset, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return clientset.NewForConfig(config)
}

func (install *installWorkflow) GetRelease(kmd *installv1alpha1.KarmadaDeployment) (*helm.Release, error) {
	if install.release == nil {
		release, err := GetRelease(install.helmClient, kmd)
		if err != nil {
			return nil, err
		}
		install.release = release
	}

	return install.release, nil
}
