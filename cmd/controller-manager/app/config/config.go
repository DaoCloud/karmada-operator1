package config

import (
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	componentbaseconfig "k8s.io/component-base/config"

	crdclientset "github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helminstaller "github.com/daocloud/karmada-operator/pkg/installer/helm"
)

type Config struct {
	Client        *clientset.Clientset
	Kubeconfig    *restclient.Config
	CRDClient     *crdclientset.Clientset
	EventRecorder record.EventRecorder

	ChartResource  *helminstaller.ChartResource
	LeaderElection componentbaseconfig.LeaderElectionConfiguration
}
