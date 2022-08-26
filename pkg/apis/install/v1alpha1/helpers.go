package v1alpha1

import (
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// HelmReleaseFailed represents the fact that find installer for the
	// karmada deployment failed.
	IntallModeFailedReason string = "IntallModeFailed"
	// HelmInitFailed represents the fact that the initialization of the Helm
	// configuration failed.
	HelmInitFailedReason string = "HelmInitFailed"
	// HelmChartFailed represents the fact that the chart download for the
	// karmada deployment failed.
	HelmChartFailedReason string = "HelmChartFailed"
	// HelmReleaseFailed represents the fact that the release install or upgrade for the
	// karmada deployment failed.
	HelmReleaseFailedReason string = "HelmReleaseFailed"
	// ReconciliationFailedReason represents the fact that
	// the reconciliation failed.
	ReconciliationFailedReason string = "ReconciliationFailed"
	// ReconciliationSucceededReason represents the fact that
	// the reconciliation succeeded.
	ReconciliationSucceededReason string = "ReconciliationSucceeded"
)

// KarmadaDeploymentProgressing registers progress toward reconciling the given KarmadaDeployment
// by setting the meta.ReadyCondition to 'Unknown' for ProgressingReason.
func KarmadaDeploymentProgressing(kmd *KarmadaDeployment) *KarmadaDeployment {
	kmd.Status.Conditions = []metav1.Condition{}
	newCondition := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionUnknown,
		Reason:  "Progressing",
		Message: "Reconciliation in progress",
	}
	apimeta.SetStatusCondition(&kmd.Status.Conditions, newCondition)
	return kmd
}

// KarmadaDeploymentNotReady registers a failed reconciliation of the given KarmadaDeployment.
func KarmadaDeploymentNotReady(kmd *KarmadaDeployment, reason, message string) *KarmadaDeployment {
	newCondition := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
	apimeta.SetStatusCondition(&kmd.Status.Conditions, newCondition)
	return kmd
}

// KarmadaDeploymentReady registers a successful reconciliation of the given KarmadaDeployment.
func KarmadaDeploymentReady(kmd *KarmadaDeployment) *KarmadaDeployment {
	newCondition := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  ReconciliationSucceededReason,
		Message: "Release reconciliation succeeded",
	}
	apimeta.SetStatusCondition(&kmd.Status.Conditions, newCondition)
	return kmd
}
