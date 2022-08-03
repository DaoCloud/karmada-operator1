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
	"net"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/constants"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	helmv3 "github.com/daocloud/karmada-operator/pkg/helm/v3"
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

	klog.V(5).Info("[helm-installer]:the endpoint kubeconfig info: %s", string(kubeconfig))

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
	if exist, _ := PathExists(constants.KubeconfigBasePath); !exist {
		if err := os.MkdirAll(constants.KubeconfigBasePath, 0760); err != nil {
			return "", err
		}
	}

	fileName := filepath.Join(constants.KubeconfigBasePath, kmdName)
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

// GenerateKubeconfigByInCluster load serviceaccount info under
// "/var/run/secrets/kubernetes.io/serviceaccount" and generate kubeconfig str.
func GenerateKubeconfigInCluster() ([]byte, error) {
	const (
		tokenFile  = "/var/run/secrets/kubernetes.io/serviceaccount/token"
		rootCAFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	)

	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return nil, errors.New("unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}

	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}

	caCert, err := ioutil.ReadFile(rootCAFile)
	if err != nil {
		return nil, err
	}

	config := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"kubernetes": {
				Server:                   "https://" + net.JoinHostPort(host, port),
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"karmada-operator@kubernetes": {
				Cluster:  "kubernetes",
				AuthInfo: "karmada-operator",
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"karmada-operator": {
				Token: string(token),
			},
		},
		CurrentContext: "karmada-operator@kubernetes",
	}

	return clientcmd.Write(config)
}

func GetRelease(helmClient helm.Client, kmd *installv1alpha1.KarmadaDeployment) (*helm.Release, error) {
	return helmClient.Get(fmt.Sprintf("karmada-%s", kmd.Name), helm.GetOptions{Namespace: kmd.Namespace})
}

func GetComponentRelease(helmClient helm.Client, kmd *installv1alpha1.KarmadaDeployment) (*helm.Release, error) {
	return helmClient.Get(fmt.Sprintf("karmada-%s-component", kmd.Name), helm.GetOptions{Namespace: kmd.Namespace})
}
