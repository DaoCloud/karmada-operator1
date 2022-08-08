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

package options

import (
	"github.com/daocloud/karmada-operator/cmd/app/config"
	crdclientset "github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	helminstaller "github.com/daocloud/karmada-operator/pkg/installer/helm"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/config/options"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/logs"
)

const (
	ControllerManagerUserAgent = "controller-manager"
)

type Options struct {
	LeaderElection   componentbaseconfig.LeaderElectionConfiguration
	ClientConnection componentbaseconfig.ClientConnectionConfiguration

	Logs *logs.Options

	Master       string
	Kubeconfig   string
	ChartName    string
	ChartRepoURL string
	ChartVersion string
}

func NewOptions() (*Options, error) {
	var (
		leaderElection   componentbaseconfigv1alpha1.LeaderElectionConfiguration
		clientConnection componentbaseconfigv1alpha1.ClientConnectionConfiguration
	)
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&leaderElection)
	componentbaseconfigv1alpha1.RecommendedDefaultClientConnectionConfiguration(&clientConnection)

	leaderElection.ResourceName = "karmada-operator"
	leaderElection.ResourceNamespace = "karmada-operator-system"
	leaderElection.ResourceLock = resourcelock.LeasesResourceLock

	clientConnection.ContentType = runtime.ContentTypeJSON

	var options Options

	// not need scheme.Convert
	if err := componentbaseconfigv1alpha1.Convert_v1alpha1_LeaderElectionConfiguration_To_config_LeaderElectionConfiguration(&leaderElection, &options.LeaderElection, nil); err != nil {
		return nil, err
	}
	if err := componentbaseconfigv1alpha1.Convert_v1alpha1_ClientConnectionConfiguration_To_config_ClientConnectionConfiguration(&clientConnection, &options.ClientConnection, nil); err != nil {
		return nil, err
	}

	options.Logs = logs.NewOptions()
	return &options, nil
}

func (o *Options) Flags() cliflag.NamedFlagSets {
	var fss cliflag.NamedFlagSets

	genericfs := fss.FlagSet("generic")
	genericfs.StringVar(&o.ClientConnection.ContentType, "kube-api-content-type", o.ClientConnection.ContentType, "Content type of requests sent to apiserver.")
	genericfs.Float32Var(&o.ClientConnection.QPS, "kube-api-qps", o.ClientConnection.QPS, "QPS to use while talking with kubernetes apiserver.")
	genericfs.Int32Var(&o.ClientConnection.Burst, "kube-api-burst", o.ClientConnection.Burst, "Burst to use while talking with kubernetes apiserver.")
	genericfs.StringVar(&o.ChartRepoURL, "chart-repo-url", o.ChartRepoURL, "helm repo to karmada chart")
	genericfs.StringVar(&o.ChartName, "chart-name", o.ChartName, "helm repo to karmada chart")
	genericfs.StringVar(&o.ChartVersion, "chart-version", o.ChartVersion, "chart version to karmada")

	options.BindLeaderElectionFlags(&o.LeaderElection, genericfs)

	fs := fss.FlagSet("misc")
	fs.StringVar(&o.Master, "master", o.Master, "The address of the Kubernetes API server (overrides any value in kubeconfig).")
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")

	// Set klog flags
	logsFlagSet := fss.FlagSet("logs")
	o.Logs.AddFlags(logsFlagSet)
	return fss
}

func (o *Options) Validate() error {
	return nil
}

func (o *Options) Config() (*config.Config, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags(o.Master, o.Kubeconfig)
	if err != nil {
		return nil, err
	}

	kubeconfig.ContentConfig.AcceptContentTypes = o.ClientConnection.AcceptContentTypes
	kubeconfig.ContentConfig.ContentType = o.ClientConnection.ContentType
	kubeconfig.QPS = o.ClientConnection.QPS
	kubeconfig.Burst = int(o.ClientConnection.Burst)

	client, err := clientset.NewForConfig(restclient.AddUserAgent(kubeconfig, ControllerManagerUserAgent))
	if err != nil {
		return nil, err
	}
	crdclient, err := crdclientset.NewForConfig(restclient.AddUserAgent(kubeconfig, ControllerManagerUserAgent))
	if err != nil {
		return nil, err
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: client.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: ControllerManagerUserAgent})

	chartResource := &helminstaller.ChartResource{
		Name:    o.ChartName,
		Version: o.ChartVersion,
		RepoURL: o.ChartRepoURL,
	}

	return &config.Config{
		Client:         client,
		CRDClient:      crdclient,
		Kubeconfig:     kubeconfig,
		EventRecorder:  eventRecorder,
		ChartResource:  chartResource,
		LeaderElection: o.LeaderElection,
	}, nil
}
