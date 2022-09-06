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
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/constants"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	"github.com/daocloud/karmada-operator/pkg/status"
	"github.com/daocloud/karmada-operator/pkg/utils"
)

var (
	ReleaseExistErrMsg = "cannot re-use a name that is still in use"
)

const (
	DefaulTimeout  = time.Minute * 3
	WaitPodTimeout = time.Second * 60
)

type installWorkflow struct {
	release          *helm.Release
	componentRelease *helm.Release
	values           *Values
	chartPath        string
	destHost         string

	client        clientset.Interface
	helmClient    helm.Client
	kmdClient     versioned.Interface
	destClient    clientset.Interface
	chartResource *ChartResource
}

func NewInstallWorkflow(destHost string, helmClient helm.Client, kmdClient versioned.Interface,
	destClient clientset.Interface, client clientset.Interface,
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
	case "", installv1alpha1.PreflightPhase, installv1alpha1.DeployingPhase:
		if err = install.Preflight(kmd); err != nil {
			return err
		}
		if err := install.Deploy(kmd); err != nil {
			return err
		}

		fallthrough
	case installv1alpha1.WaitingPhase:
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
	klog.InfoS("[helm-installer] start proflight phase", "kmd", kmd.Name)

	var err error
	if err = status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.PreflightPhase); err != nil {
		return err
	}

	install.chartPath, _, err = fetchChart(install.helmClient, install.chartResource)
	if err != nil {
		klog.ErrorS(err, "[helm-installer]:failed to fetch karmada chart pkg")
		kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmChartFailedReason, err.Error())
		status.SetStatus(install.kmdClient, kmd)
		return err
	}

	if install.values == nil {
		values := Compose(kmd)
		IPs, err := utils.GetKubeMasterIP(install.client)
		if err != nil {
			klog.ErrorS(err, "[helm-installer]:failed get ips of kubenetes master node")
		}

		SetChartDefaultValues(values, kmd.Spec.ControlPlane.Namespace, IPs)
		install.values = values
	}
	return nil
}

func fetchChart(helmClient helm.Client, source *ChartResource) (string, bool, error) {
	repoPath, filename, err := makeChartPath(constants.ChartBasePath, source)
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

func (install *installWorkflow) Deploy(kmd *installv1alpha1.KarmadaDeployment) error {
	klog.InfoS("[helm-installer]:start deploy phase", "kmd", kmd.Name)

	if err := status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.DeployingPhase); err != nil {
		return err
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

	values, err := install.values.ValuesWithHostInstallMode()
	klog.V(5).Infof("chart values.ymal:\n%s", values)

	if err != nil {
		klog.ErrorS(err, "[helm-installer]:failed to compose chart values")
		kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmInitFailedReason, err.Error())
		status.SetStatus(install.kmdClient, kmd)
		return err
	}
	releaseName := fmt.Sprintf("karmada-%s", kmd.Name)
	install.release, err = install.helmClient.UpgradeFromPath(install.chartPath,
		releaseName, values, helm.UpgradeOptions{
			Namespace:         ns.Name,
			Timeout:           DefaulTimeout,
			Install:           true,
			Force:             true,
			SkipCRDs:          false,
			Wait:              false,
			Atomic:            true,
			DisableValidation: true,
		})

	if err != nil && !strings.Contains(err.Error(), ReleaseExistErrMsg) {
		klog.ErrorS(err, "[helm-installer]:failed to install karmada chart", "kmd", kmd.Name)
		kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmReleaseFailedReason, err.Error())
		status.SetStatus(install.kmdClient, kmd)
		return err
	}

	// TODO: if community supports installing components together, it will not need install again.
	// deploy karmada component
	if len(install.values.Components) > 0 {
		values, err := install.values.ValuesWithComponentInstallMode()
		if err != nil {
			klog.ErrorS(err, "[helm-installer]:failed to compose chart values")
			kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmInitFailedReason, err.Error())
			install.kmdClient.InstallV1alpha1().KarmadaDeployments().UpdateStatus(context.Background(), kmd, metav1.UpdateOptions{})
			return err
		}
		crn := fmt.Sprintf("karmada-%s-component", kmd.Name)
		install.componentRelease, err = install.helmClient.UpgradeFromPath(install.chartPath,
			crn, values, helm.UpgradeOptions{
				Namespace:         ns.Name,
				Timeout:           DefaulTimeout,
				Install:           true,
				Force:             true,
				SkipCRDs:          false,
				Wait:              false,
				Atomic:            true,
				DisableValidation: true,
			})
		if err != nil {
			klog.ErrorS(err, "[helm-installer]:failed to install karmada component", "kmd", kmd.Name)
			kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmReleaseFailedReason, err.Error())
			status.SetStatus(install.kmdClient, kmd)
			return err
		}
	}

	return nil
}

func (install *installWorkflow) Wait(kmd *installv1alpha1.KarmadaDeployment) error {
	klog.InfoS("[helm-installer]:start wait phase", "kmd", kmd.Name)
	release, err := install.GetRelease(kmd)
	if err != nil {
		kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.HelmReleaseFailedReason, err.Error())
		status.SetStatus(install.kmdClient, kmd)
		return err
	}

	kmd.Status.Phase = installv1alpha1.WaitingPhase
	if err := status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.WaitingPhase); err != nil {
		return err
	}

	karmadaApiserver := fmt.Sprintf("%s-karmada-apiserver", release.Name)
	for _, m := range []string{karmadaApiserver, "etcd"} {
		if _, err := waitForPodReady(install.client, release.Namespace, m); err != nil {
			klog.ErrorS(err, "[helm-installer]:failed to wait ready", "component", m, "namespace", release.Namespace)
			kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.ReconciliationFailedReason, err.Error())
			status.SetStatus(install.kmdClient, kmd)
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
	klog.InfoS("[helm-installer]:start completed phase", "kmd", kmd.Name)
	release, err := install.GetRelease(kmd)
	if err != nil {
		return err
	}
	secretmapName := fmt.Sprintf("%s-kubeconfig", release.Name)
	secret, err := install.destClient.CoreV1().Secrets(release.Namespace).Get(context.TODO(), secretmapName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// expose the karmada service
	configByte, exist := secret.Data["kubeconfig"]
	if !exist {
		return fmt.Errorf("failed to load internal kubeconfig from secret %s", secret.Name)
	}

	if kmd.Spec.ControlPlane.ServiceType != corev1.ServiceTypeClusterIP {
		configByte, err = install.BuildExtetnalKubeconfig(configByte)
		if err != nil {
			return err
		}

		// set karmada version and kube apiserver version to kmd status.
		// the karmada version is by default.
		version, err := GetKarmadaVersion(configByte)
		if err != nil {
			klog.ErrorS(err, "[helm-installer]:failed get karmada version")
		}

		kmd.Status.KubernetesVersion = version.String()
		klog.InfoS("[helm-installer]:success install karmada release", "version", version.String())
	}

	// TODO: How to get the karmada version?
	kmd.Status.KarmadaVersion = constants.DefaultKarmadaVersion

	secretCopy, err := CreateSecretForExternalKubeconfig(install.client, configByte, kmd)
	if err != nil {
		klog.ErrorS(err, "[helm-installer]:failed create secret for external kubeconfig")
		return err
	}

	// The flow is completed, set the controlPlaneReady.
	kmd.Status.SecretRef = &installv1alpha1.LocalSecretReference{
		Name:      secretCopy.Name,
		Namespace: secretCopy.Namespace,
	}

	kmd.Status.ControlPlaneReady = true
	return status.SetStatusPhase(install.kmdClient, kmd, installv1alpha1.ControlPlaneReadyPhase)
}

func (install *installWorkflow) GetRelease(kmd *installv1alpha1.KarmadaDeployment) (*helm.Release, error) {
	if install.release == nil {
		release, err := GetRelease(install.helmClient, kmd)
		if release == nil {
			return nil, fmt.Errorf("failed to find the release: %s", fmt.Sprintf("karmada-%s", kmd.Name))
		}
		if err != nil {
			return nil, err
		}
		install.release = release
	}

	return install.release, nil
}

func (install *installWorkflow) BuildExtetnalKubeconfig(internalConfig []byte) ([]byte, error) {
	clusterName := fmt.Sprintf("%s-apiserver", install.release.Name)
	config, err := clientcmd.Load(internalConfig)
	if err != nil {
		return nil, err
	}

	materIP, err := utils.GetKubeMasterIP(install.client)
	if err != nil {
		return nil, err
	}

	port, err := install.ServicePort()
	if err != nil {
		klog.ErrorS(err, "failed to get karmada apiserver service port.")
		return nil, err
	}

	serverURL := fmt.Sprintf("https://%s:%v", materIP[0], port)
	if cluster, exist := config.Clusters[clusterName]; exist {
		cluster.Server = serverURL
	}

	return clientcmd.Write(*config)
}

func (install *installWorkflow) ServicePort() (int32, error) {
	namespace := install.release.Namespace
	serviceName := fmt.Sprintf("%s-apiserver", install.release.Name)
	service, err := install.destClient.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}

	var port int32
	ports := service.Spec.Ports
	for _, p := range ports {
		if p.Name != serviceName {
			continue
		}

		if service.Spec.Type == corev1.ServiceTypeClusterIP {
			port = p.Port
		} else {
			port = p.NodePort
		}
	}

	return port, nil
}

func GetKarmadaVersion(kubeconfig []byte) (*version.Info, error) {
	client, err := utils.NewClientForKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return client.ServerVersion()
}

func CreateSecretForExternalKubeconfig(client clientset.Interface, data []byte, kmd *installv1alpha1.KarmadaDeployment) (*corev1.Secret, error) {
	// restore the karmda kubeconfig to secret, and set the secret info to kmd status.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-kubeconfig-", kmd.Name),
			Namespace:    kmd.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": data,
		},
		Type: corev1.SecretTypeOpaque,
	}

	// resote the secret in current namespace
	currentNamespace := utils.GetCurrentNSOrDefault()
	secret, err := client.CoreV1().Secrets(currentNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		secret, err = client.CoreV1().Secrets(kmd.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}
	return secret, nil
}
