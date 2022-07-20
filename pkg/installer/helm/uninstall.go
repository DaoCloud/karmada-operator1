package helm

import installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"

func (helm *HelmInstaller) Uninstall(kd *installv1alpha1.KarmadaDeployment) error {
	return nil
}
