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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	helmv3 "github.com/daocloud/karmada-operator/pkg/helm/v3"
)

const (
	// KubeconfigBasePath = "/var/run/karmada-operator/kubeconfig"
	KubeconfigBasePath = "/var/run/karmada-operator/config"
)

var (
	versionedLogger log.Logger
)

func init() {
	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	versionedLogger = log.With(logger, "component", "helm", "version", "v3")
}

type ChartResource struct {
	Name    string
	RepoURL string
	Version string
}

type HelmInstaller struct {
	*installWorkflow
	*uninstallWorkflow
}

func NewHelmInstaller(kmd *installv1alpha1.KarmadaDeployment, kmdClient versioned.Interface, client clientset.Interface, chartResource *ChartResource) (*HelmInstaller, error) {
	if kmd.Spec.ControlPlane.EndPointCfg == nil {
		return nil, fmt.Errorf("failed load endpoint config")
	}

	// TODO: load kubeconfig from kubeconfig path
	// is the secret data to kubeconfig file or caData and token.
	secretRef := kmd.Spec.ControlPlane.EndPointCfg.SecretRef
	secret, err := client.CoreV1().Secrets(secretRef.Namespace).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	kubeconfig := secret.Data["kubeconfig"]
	kubeconfigPath, err := WriteKubeconfig(kubeconfig, kmd.Name)
	if err != nil {
		klog.Errorf("[helm-installer]:failed to write the kubeconfig to directory %s for %s: %v:", kubeconfigPath, kmd.Name, err)
		return nil, err
	}

	config, err := BuildClusterKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	destClient, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	helmClient := helmv3.New(versionedLogger, kubeconfigPath, config)

	return &HelmInstaller{
		installWorkflow:   NewInstallWorkflow(config.APIPath, helmClient, kmdClient, destClient, client, chartResource),
		uninstallWorkflow: NewUninstallWorkflow(client, destClient, helmClient),
	}, nil
}

// WriteKubeconfig write kubeconfig to the directory "/var/run/karmada-operator"
// this is a bug to helm client.
func WriteKubeconfig(kubeconfig []byte, kmdName string) (string, error) {
	if exist, _ := PathExists(KubeconfigBasePath); !exist {
		if err := os.MkdirAll(KubeconfigBasePath, 0760); err != nil {
			return "", err
		}
	}

	fileName := filepath.Join(KubeconfigBasePath, kmdName)
	if err := ioutil.WriteFile(fileName, kubeconfig, 0660); err != nil {
		return "", err
	}
	return fileName, nil
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// BuildClusterKubeconfig is to build the install cluster kubeconfig
// the kubeconfig is from two way, one is be restored to secret, and
// other is specified path of kubeconfig.
func BuildClusterKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	klog.V(5).Infof("kubeconfig data: %s", string(kubeconfig))
	config, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		klog.Errorf("[helm-installer]:failed to build the target install cluster kubeconfig, please check the secret: %v:", err)
		return nil, err
	}
	return config.ClientConfig()
}

func GetRelease(helmClient helm.Client, kmd *installv1alpha1.KarmadaDeployment) (*helm.Release, error) {
	return helmClient.Get(fmt.Sprintf("karmada-%s", kmd.Name), helm.GetOptions{Namespace: kmd.Namespace})
}
