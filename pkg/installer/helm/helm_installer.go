package helm

import (
	helm "github.com/daocloud/karmada-operator/pkg/helm"
)

type HelmInstaller struct {
	helmClient helm.Client
}

func (helm *HelmInstaller) Install() {
}

func (helm *HelmInstaller) Uninstall() {
}
