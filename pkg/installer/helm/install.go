package helm

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	helm "github.com/daocloud/karmada-operator/pkg/helm"
	"k8s.io/client-go/kubernetes"
)

const (
	chartBasePath    = "/run/var"
	KarmadaNamespace = "karmada-system"
	DefaulTimeout    = time.Second * 120
)

var (
	KarmadaImage    = []string{"schedulerEstimator", "descheduler", "search", "scheduler", "webhook", "controllerManager", "agent", "aggregatedApiServer"}
	KubernatesImage = []string{"etcd", "apiServer", "kubeControllerManager"}
)

func (installer *HelmInstaller) Install(kd *installv1alpha1.KarmadaDeployment) error {
	preflight := &PreflightPhase{installer.helmClient}
	deploy, err := preflight.Preflight(installer.clientset, installer.chartResource, kd)
	if err != nil {
		return err
	}

	wait, err := deploy.Deploy(kd)
	if err != nil {
		return err
	}

	return wait.Wait()
}

type PreflightGetter interface {
	Preflight() (DeployGetter, error)
}

type PreflightPhase struct {
	helmClient helm.Client
}

func (p *PreflightPhase) Preflight(clientset kubernetes.Interface, source *ChartResource, kd *installv1alpha1.KarmadaDeployment) (DeployGetter, error) {
	chartPath, _, err := p.FetchChart(source)
	if err != nil {
		return nil, err
	}
	values, err := p.ComposeValues(kd)
	if err != nil {
		return nil, err
	}

	return &DeployPhase{
		clientset:  clientset,
		helmClient: p.helmClient,
		values:     values,
		chartPath:  chartPath,
	}, nil
}

func (p *PreflightPhase) FetchChart(source *ChartResource) (string, bool, error) {
	repoPath, filename, err := makeChartPath(chartBasePath, source)
	if err != nil {
		return "", false, err
	}
	chartPath := filepath.Join(repoPath, filename)
	stat, err := os.Stat(chartPath)
	switch {
	case os.IsNotExist(err):
		chartPath, err = p.helmClient.PullWithRepoURL("nil", "", "", "")
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
	repoPath := filepath.Join(base, base64.URLEncoding.EncodeToString([]byte(source.CleanRepoURL())))
	if err := os.MkdirAll(repoPath, 00750); err != nil {
		return "", "", err
	}
	return repoPath, fmt.Sprintf("%s-%s.tgz", source.Name, source.Version), nil
}

func (p *PreflightPhase) ComposeValues(kd *installv1alpha1.KarmadaDeployment) ([]byte, error) {
	values := helm.Values{}

	var (
		etcdStorageModeKey  = "etcd.internal.storageType"
		etcdStorageClassKey = "etcd.internal.pvc.storageClass"
		etcdStorageSize     = "etcd.internal.pvc.size"
	)

	controlPlane := kd.Spec.ControlPlane
	if controlPlane.ETCD != nil {
		if len(controlPlane.ETCD.StorageMode) > 0 {
			values[etcdStorageModeKey] = controlPlane.ETCD.StorageMode
		}
		if len(controlPlane.ETCD.StorageClass) > 0 {
			values[etcdStorageClassKey] = controlPlane.ETCD.StorageClass
		}
		// TODO: Validation
		if len(controlPlane.ETCD.Size) > 0 {
			values[etcdStorageSize] = controlPlane.ETCD.Size
		}
	}

	for _, module := range controlPlane.Modules {
		if module.Replicas != nil {
			replicasCountKey := string(module.Name) + ".replicaCount"
			values[replicasCountKey] = module.Replicas
		}
		if len(module.Image) > 0 {
			replicasCountKey := string(module.Name) + ".image" + ".repository"
			values[replicasCountKey] = module.Image
		}
	}

	if len(controlPlane.Components) > 0 {
		values["components"] = controlPlane.Components
	}

	if kd.Spec.Images != nil {
		for _, k := range KarmadaImage {
			if len(kd.Spec.Images.KarmadaRegistry) > 0 {
				values[k+".image.registry"] = kd.Spec.Images.KarmadaRegistry
			}
			if len(kd.Spec.Images.KarmadaVersion) > 0 {
				values[k+".image.tag"] = kd.Spec.Images.KarmadaVersion
			}
		}
		for _, k := range KubernatesImage {
			if len(kd.Spec.Images.KubeResgistry) > 0 {
				values[k+".image.registry"] = kd.Spec.Images.KubeResgistry
			}
			if len(kd.Spec.Images.KubeVersion) > 0 {
				values[k+".image.tag"] = kd.Spec.Images.KubeResgistry
			}
		}
	}

	// TODO: load chart config of configmap
	return values.YAML()
}

type DeployGetter interface {
	Deploy(kd *installv1alpha1.KarmadaDeployment) (WaitGetter, error)
}

type DeployPhase struct {
	clientset  kubernetes.Interface
	helmClient helm.Client
	values     []byte
	chartPath  string
}

func (d *DeployPhase) Deploy(kd *installv1alpha1.KarmadaDeployment) (WaitGetter, error) {
	if len(kd.Spec.ControlPlane.Namespace) == 0 {
		kd.Spec.ControlPlane.Namespace = KarmadaNamespace
	}
	release, err := d.helmClient.UpgradeFromPath(d.chartPath, "karmada"+kd.Name, d.values, helm.UpgradeOptions{
		Namespace: kd.Spec.ControlPlane.Namespace,
		Timeout:   DefaulTimeout,
		Install:   true,

		// TODO: why?
		Force:             true,
		SkipCRDs:          false,
		Wait:              false,
		Atomic:            true,
		DisableValidation: false,
	})
	if err != nil {
		return nil, err
	}
	return &WaitPhase{
		release:   release,
		clientset: d.clientset,
	}, nil
}

type WaitGetter interface {
	Wait() error
}

type WaitPhase struct {
	release   *helm.Release
	clientset kubernetes.Interface
}

func (w *WaitPhase) Wait() error {
	return nil
}

func (w *WaitPhase) WaitForApiserverReady() {
}
