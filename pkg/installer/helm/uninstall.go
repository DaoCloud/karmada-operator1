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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/constants"
	"github.com/daocloud/karmada-operator/pkg/helm"
	"github.com/daocloud/karmada-operator/pkg/utils"
)

const (
	DefaultUninstallTimeOut = time.Second * 60
)

var (
	ReleaseNotLoadErrMsg = "uninstall: Release not loaded"
)

type uninstallWorkflow struct {
	client     clientset.Interface
	destClient clientset.Interface
	helmClient helm.Client
}

func NewUninstallWorkflow(client clientset.Interface, destClient clientset.Interface, helmClient helm.Client) *uninstallWorkflow {
	return &uninstallWorkflow{
		client:     client,
		destClient: destClient,
		helmClient: helmClient,
	}
}

func (un *uninstallWorkflow) Uninstall(kmd *installv1alpha1.KarmadaDeployment) error {
	if err := un.UninstallComponent(kmd); err != nil {
		klog.ErrorS(err, "[helm-installer]:failed to uninstall karmada componnets", "kmd", kmd.Name)
		return err
	}

	klog.InfoS("[helm-installer]:start uninstall phase", "kmd", kmd.Name)
	release, err := GetRelease(un.helmClient, kmd)
	if err != nil {
		return err
	}

	// if the release is deleted by user. it will skip the workflow.
	if release == nil {
		return nil
	}

	err = un.helmClient.Uninstall(release.Name, helm.UninstallOptions{
		KeepHistory: false,
		Namespace:   release.Namespace,
		Timeout:     DefaultUninstallTimeOut,
	})

	// TODO: if the karmada release is not loead, ingore the err.
	if err != nil && !strings.Contains(err.Error(), ReleaseNotLoadErrMsg) {
		klog.ErrorS(err, "[helm-installer]:failed to uninstall karmada", "kmd", kmd.Name)
		return err
	}

	return un.cleanup(kmd, release.Name, release.Namespace)
}

func (un *uninstallWorkflow) UninstallComponent(kmd *installv1alpha1.KarmadaDeployment) error {
	klog.InfoS("[helm-installer]:start uninstall conponent phase", "kmd", kmd.Name)
	release, err := GetComponentRelease(un.helmClient, kmd)
	if err != nil {
		return err
	}

	if release == nil {
		return nil
	}

	err = un.helmClient.Uninstall(release.Name, helm.UninstallOptions{
		KeepHistory: false,
		Namespace:   release.Namespace,
		Timeout:     DefaultUninstallTimeOut,
	})

	// TODO: if the karmada release is not loead, ingore the err.
	if err != nil && !strings.Contains(err.Error(), ReleaseNotLoadErrMsg) {
		klog.ErrorS(err, "[helm-installer]:failed to uninstall karmada", "kmd", kmd.Name)
		return err
	}

	return nil
}

// There are some RBAC resources that are used by the `preJob` that
// can not be deleted by the `uninstall` command above.
// 1. sa/karmada-pre-job
// 2. clusterRole/karmada-pre-job
// 3. clusterRoleBinding/karmada-pre-job
// 4. ns/karmada-system
func (un *uninstallWorkflow) cleanup(kmd *installv1alpha1.KarmadaDeployment, release, namespace string) error {
	controlPlaneNamespace := utils.GetControlPlaneNamespace(kmd)
	err := un.destClient.CoreV1().ServiceAccounts(controlPlaneNamespace).Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to delete ServiceAccount(%s/%s)", controlPlaneNamespace, fmt.Sprintf("%s-pre-job", release))
	}
	err = un.destClient.RbacV1().ClusterRoles().Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to delete ClusterRole(%s)", fmt.Sprintf("%s-pre-job", release))
	}
	err = un.destClient.RbacV1().ClusterRoleBindings().Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to delete ClusterRoleBinding(%s)", fmt.Sprintf("%s-pre-job", release))
	}
	if kmd.GetAnnotations()[constants.RandomNamespace] != "" {
		// Delete only randomly generated namespaces
		if err = un.destClient.CoreV1().Namespaces().Delete(context.TODO(), kmd.GetAnnotations()[constants.RandomNamespace], metav1.DeleteOptions{}); err != nil {
			klog.ErrorS(err, "Failed delete randomly generated namespaces")
			return err
		}
	}
	// delete the secret of karmada instance kubeconfig on the cluster.
	secretRef := kmd.Status.SecretRef
	if secretRef != nil {
		return un.client.CoreV1().Secrets(secretRef.Namespace).Delete(context.TODO(), secretRef.Name, metav1.DeleteOptions{})
	}

	// TODO: delete kubeconfig from the directory
	return nil
}
