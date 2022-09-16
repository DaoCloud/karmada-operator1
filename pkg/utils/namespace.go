package utils

import (
	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/constants"
)

func GetControlPlaneNamespace(kmd *installv1alpha1.KarmadaDeployment) string {
	if kmd.Spec.ControlPlane.Namespace != "" {
		return kmd.Spec.ControlPlane.Namespace
	}
	if ns, exist := kmd.Annotations[constants.RandomNamespace]; exist {
		return ns
	}
	return "karmada-system"
}
