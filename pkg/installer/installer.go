package installer

import (
	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
)

type Interface interface {
	Install(kd *installv1alpha1.KarmadaDeployment) error
	Uninstall(kd *installv1alpha1.KarmadaDeployment) error
}

type installerFactory struct{}

func (factory *installerFactory) InitInstaller(kd *installv1alpha1.KarmadaDeployment) Interface {
	return nil
}
