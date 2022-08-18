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

package installer

import (
	"fmt"

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helminstaller "github.com/daocloud/karmada-operator/pkg/installer/helm"
	"github.com/daocloud/karmada-operator/pkg/status"
)

type Interface interface {
	Install(kd *installv1alpha1.KarmadaDeployment) error
	Uninstall(kd *installv1alpha1.KarmadaDeployment) error
}

type InstallerFactory struct {
	kmdClient     versioned.Interface
	clientset     clientset.Interface
	chartResource *helminstaller.ChartResource

	installers map[string]Interface
}

func NewInstallerFactory(kmdClient versioned.Interface, clientset clientset.Interface, chartResource *helminstaller.ChartResource) *InstallerFactory {
	return &InstallerFactory{
		kmdClient:     kmdClient,
		clientset:     clientset,
		chartResource: chartResource,
		installers:    make(map[string]Interface),
	}
}

type Action string

const (
	InstallAction   Action = "install"
	UninstallAction Action = "uninstall"
	UpgradeAction   Action = "upgrade"
	RollBackAction  Action = "rollback"
)

func (factory *InstallerFactory) SyncWithAction(kmd *installv1alpha1.KarmadaDeployment, action Action) error {
	installer, err := factory.GetInstaller(kmd)
	if err != nil {
		kmd = installv1alpha1.KarmadaDeploymentNotReady(kmd, installv1alpha1.IntallModeFailedReason, err.Error())
		status.SetStatus(factory.kmdClient, kmd)
		return err
	}

	klog.V(4).Infof("start %v workflow for %s", action, kmd.Name)
	switch action {
	case InstallAction:
		return installer.Install(kmd)
	case UninstallAction:
		return installer.Uninstall(kmd)
	case UpgradeAction:
		// TODO:
	case RollBackAction:
		// TODO:
	default:
		return fmt.Errorf("unable to recognize the action: %s", action)
	}
	return nil
}

func (factory *InstallerFactory) Sync(kmd *installv1alpha1.KarmadaDeployment) error {
	// Observe KarmadaDeployment generation.
	if kmd.Status.ObservedGeneration != kmd.Generation {
		kmd.Status.ObservedGeneration = kmd.Generation
		kmd = installv1alpha1.KarmadaDeploymentProgressing(kmd)
		status.SetStatus(factory.kmdClient, kmd)
	}
	// TODO: automatically calculate the action
	err := factory.SyncWithAction(kmd, InstallAction)
	if err == nil {
		kmd = installv1alpha1.KarmadaDeploymentReady(kmd)
		return status.SetStatus(factory.kmdClient, kmd)
	}
	return err
}

func (factory *InstallerFactory) GetInstaller(kmd *installv1alpha1.KarmadaDeployment) (Interface, error) {
	var err error
	var installer Interface
	switch *kmd.Spec.Mode {
	case "", installv1alpha1.HelmMode:
		installer, err = helminstaller.NewHelmInstaller(kmd, factory.kmdClient, factory.clientset, factory.chartResource)
	case installv1alpha1.KarmadactlMode:
		// TODO: karmadactl installer
		return nil, fmt.Errorf("the %s install mode is unimplemented", *kmd.Spec.Mode)
	default:
		return nil, fmt.Errorf("unknown install mode: %s", *kmd.Spec.Mode)
	}

	if err != nil {
		return nil, err
	}

	return installer, nil
}
