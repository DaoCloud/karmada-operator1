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
	"k8s.io/client-go/kubernetes"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/helm"
)

const (
	DefaultUninstallTimeOut = time.Second * 60
)

var (
	ReleaseNotLoadErrMsg = "uninstall: Release not loaded"
)

type uninstallWorkflow struct {
	destClient kubernetes.Interface
	helmClient helm.Client
}

func NewUninstallWorkflow(destClient kubernetes.Interface, helmClient helm.Client) *uninstallWorkflow {
	return &uninstallWorkflow{
		destClient: destClient,
		helmClient: helmClient,
	}
}

func (un *uninstallWorkflow) Uninstall(kmd *installv1alpha1.KarmadaDeployment) error {
	release, err := GetRelease(un.helmClient, kmd)
	if err != nil {
		return err
	}

	err = un.helmClient.Uninstall(release.Name, helm.UninstallOptions{
		KeepHistory: false,
		Namespace:   release.Namespace,
		Timeout:     DefaultUninstallTimeOut,
	})

	// TODO: if the karmada release is not loead, ingore the err.
	if err != nil && !strings.Contains(err.Error(), ReleaseNotLoadErrMsg) {
		return err
	}

	return un.cleanup(kmd, release.Name, release.Namespace)
}

// There are some RBAC resources that are used by the `preJob` that
// can not be deleted by the `uninstall` command above.
// 1. sa/karmada-pre-job
// 2. clusterRole/karmada-pre-job
// 3. clusterRoleBinding/karmada-pre-job
// 4. ns/karmada-system
func (un *uninstallWorkflow) cleanup(kd *installv1alpha1.KarmadaDeployment, release, namespace string) error {
	_ = un.destClient.CoreV1().ServiceAccounts(kd.Spec.ControlPlane.Namespace).Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})

	_ = un.destClient.RbacV1().ClusterRoles().Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})

	_ = un.destClient.RbacV1().ClusterRoleBindings().Delete(
		context.TODO(), fmt.Sprintf("%s-pre-job", release), metav1.DeleteOptions{})

	_ = un.destClient.CoreV1().Namespaces().Delete(
		context.TODO(), namespace, metav1.DeleteOptions{})

	return nil
}
