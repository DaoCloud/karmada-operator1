package helm

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	helmv3 "github.com/daocloud/karmada-operator/pkg/helm/v3"
	"github.com/go-kit/kit/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
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

// CleanRepoURL returns the RepoURL but removes the query string and ensures
// it ends with a trailing slash.
func (c ChartResource) CleanRepoURL() string {
	cleanURL, err := url.Parse(c.RepoURL)
	if err != nil {
		return strings.TrimSuffix(c.RepoURL, "/") + "/"
	}
	cleanURL.Path = strings.TrimSuffix(cleanURL.Path, "/") + "/"
	cleanURL.RawQuery = ""
	return cleanURL.String()
}

type HelmInstaller struct {
	helmClient    helm.Client
	clientset     kubernetes.Interface
	chartResource *ChartResource
}

func NewHelmInstaller(kd *installv1alpha1.KarmadaDeployment, clientset kubernetes.Interface, chartResource *ChartResource) (*HelmInstaller, error) {
	config, err := BuildKubeconfig(kd.Spec.ControlPlane.EndPointCfg, clientset)
	if err != nil {
		return nil, err
	}
	client := helmv3.New(versionedLogger, config)

	return &HelmInstaller{
		helmClient:    client,
		clientset:     clientset,
		chartResource: chartResource,
	}, nil
}

func BuildKubeconfig(endPointcfg *installv1alpha1.EndPointCfg, clientset kubernetes.Interface) (*rest.Config, error) {
	if endPointcfg == nil {
		return nil, fmt.Errorf("Failed load endpoint config")
	}

	// TODO: load kubeconfig from kubeconfig path
	secretRef := endPointcfg.SecretRef
	secret, err := clientset.CoreV1().Secrets(secretRef.Namespace).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	kubeconfig := secret.Data["kubeconfig"]
	config, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, err
	}
	return config.ClientConfig()
}
