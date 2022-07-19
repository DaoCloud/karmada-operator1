package installer

import (
	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	helminstaller "github.com/daocloud/karmada-operator/pkg/installer/helm"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Install(kd *installv1alpha1.KarmadaDeployment) error
	Uninstall(kd *installv1alpha1.KarmadaDeployment) error
}

func InitInstaller(kd *installv1alpha1.KarmadaDeployment, clientset kubernetes.Interface, chartResource *helminstaller.ChartResource) (Interface, error) {
	switch *kd.Spec.Mode {
	case installv1alpha1.ChartMode:
		return helminstaller.NewHelmInstaller(kd, clientset, chartResource)
	// TODO: support more installer
	default:
		return helminstaller.NewHelmInstaller(kd, clientset, chartResource)
	}
}
