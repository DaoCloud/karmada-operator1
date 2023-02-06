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

package app

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/term"
	"k8s.io/klog/v2"

	"github.com/daocloud/karmada-operator/cmd/app/config"
	"github.com/daocloud/karmada-operator/cmd/app/options"
	"github.com/daocloud/karmada-operator/pkg/controller"
	summary "github.com/daocloud/karmada-operator/pkg/controller/summary"
	"github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	"github.com/daocloud/karmada-operator/pkg/generated/informers/externalversions"
	"github.com/daocloud/karmada-operator/pkg/version"
	"github.com/daocloud/karmada-operator/pkg/version/verflag"
)

func NewControllerManagerCommand() *cobra.Command {
	opts, _ := options.NewOptions()
	cmd := &cobra.Command{
		Use: "karmada-operator",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// k8s.io/kubernetes/cmd/kube-controller-manager/app/controllermanager.go

			// silence client-go warnings.
			// controller-manager generically watches APIs (including deprecated ones),
			// and CI ensures it works properly against matching kube-apiserver versions.
			restclient.SetDefaultWarningHandler(restclient.NoWarnings{})
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			cliflag.PrintFlags(cmd.Flags())

			config, err := opts.Config()
			if err != nil {
				return err
			}

			if err := Run(config.Complete(), wait.NeverStop); err != nil {
				return err
			}
			return nil
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}

	namedFlagSets := opts.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name(), logs.SkipLoggingConfigurationFlags())

	fs := cmd.Flags()
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, namedFlagSets, cols)
	return cmd
}

func Run(c *config.CompletedConfig, stopCh <-chan struct{}) error {
	// To help debugging, immediately log version
	klog.InfoS("Operator Version", "version", version.Get())
	klog.InfoS("Golang settings", "GOGC", os.Getenv("GOGC"), "GOMAXPROCS", os.Getenv("GOMAXPROCS"), "GOTRACEBACK", os.Getenv("GOTRACEBACK"))

	if !c.LeaderElection.LeaderElect {
		return StartControllers(c, stopCh)
	}

	id, err := os.Hostname()
	if err != nil {
		return err
	}

	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())

	rl, err := resourcelock.NewFromKubeconfig(
		c.LeaderElection.ResourceLock,
		c.LeaderElection.ResourceNamespace,
		c.LeaderElection.ResourceName,
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: c.EventRecorder,
		},
		c.Kubeconfig,
		c.LeaderElection.RenewDeadline.Duration,
	)
	if err != nil {
		return fmt.Errorf("failed to create resource lock: %w", err)
	}

	leaderelection.RunOrDie(context.TODO(), leaderelection.LeaderElectionConfig{
		Name: c.LeaderElection.ResourceName,

		Lock:          rl,
		LeaseDuration: c.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: c.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   c.LeaderElection.RetryPeriod.Duration,

		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(_ context.Context) {
				_ = StartControllers(c, stopCh)
			},
			OnStoppedLeading: func() {
				klog.InfoS("leaderelection lost")
			},
		},
	})

	return nil
}

func StartControllers(c *config.CompletedConfig, stopCh <-chan struct{}) error {
	client, err := kubernetes.NewForConfig(c.Kubeconfig)
	if err != nil {
		return err
	}
	kmdClient, err := versioned.NewForConfig(c.Kubeconfig)
	if err != nil {
		return err
	}

	informerFactory := externalversions.NewSharedInformerFactory(kmdClient, 0)
	informers := informerFactory.Install().V1alpha1().KarmadaDeployments()

	operator := controller.NewController(kmdClient, client, c.ChartResource, informers)
	summary := summary.NewController(client, kmdClient, informers)

	informerFactory.Start(stopCh)

	go operator.Run(1, stopCh)
	go summary.Run(1, stopCh)

	<-stopCh
	return nil
}
