package app

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/term"
	"k8s.io/klog/v2"

	"github.com/daocloud/karmada-operator/cmd/controller-manager/app/config"
	"github.com/daocloud/karmada-operator/cmd/controller-manager/app/options"
	"github.com/daocloud/karmada-operator/pkg/controller"
	clientset "github.com/daocloud/karmada-operator/pkg/generated/clientset/versioned"
	"github.com/daocloud/karmada-operator/pkg/generated/informers/externalversions"
	"github.com/daocloud/karmada-operator/pkg/version/verflag"
)

func NewControllerManagerCommand() *cobra.Command {
	opts, _ := options.NewControllerManagerOptions()
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

			if err := Run(config); err != nil {
				return err
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

func Run(c *config.Config) error {
	if !c.LeaderElection.LeaderElect {
		return RunManager(c.Kubeconfig, wait.NeverStop)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("unable to get hostname: %v", err)
	}
	id := hostname + "_" + string(uuid.NewUUID())

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
			OnStartedLeading: func(ctx context.Context) {
				_ = RunManager(c.Kubeconfig, wait.NeverStop)
			},
			OnStoppedLeading: func() {
				klog.Info("leaderelection lost")
			},
		},
	})
	return nil
}

func RunManager(config *rest.Config, stopCh <-chan struct{}) error {
	client, err := clientset.NewForConfig(config)
	if err != nil {
		return err
	}

	informerFactory := externalversions.NewSharedInformerFactory(client, 0)
	informers := informerFactory.Install().V1alpha1().KarmadaDeployments()
	controller := controller.NewController(client, informers)

	informerFactory.Start(stopCh)
	if !cache.WaitForCacheSync(stopCh, informers.Informer().HasSynced) {
		klog.Errorf("Failed to wait for cache sync")
	}

	klog.Infof("informer caches is synced: %v")

	go controller.Run(1, stopCh)

	<-stopCh
	return nil
}
