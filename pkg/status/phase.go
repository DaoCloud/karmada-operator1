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

package status

import (
	"context"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

func SetStatus(client versioned.Interface, kmd *installv1alpha1.KarmadaDeployment) error {
	older := kmd.DeepCopy()
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newer, err := client.InstallV1alpha1().KarmadaDeployments().Get(context.TODO(), kmd.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		newer.Status = older.Status

		_, err = client.InstallV1alpha1().KarmadaDeployments().UpdateStatus(context.TODO(), newer, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
}
