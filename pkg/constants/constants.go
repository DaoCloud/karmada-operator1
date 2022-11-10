package constants

const (
	// Default karmada chart install system.
	DefaultKarmadaVersion = "v1.3.0"
	// Default karmada chart install system.
	KarmadaNamespace = "karmada-system"
	// karmada apiserver default port.
	KarmadaAPIServerNodePort = 32443
	// karmada chart download dir.
	ChartBasePath = "/var/run/karmada-operator"
	// KubeconfigBasePath = "/var/run/karmada-operator/kubeconfig".
	KubeconfigBasePath = ChartBasePath + "/config"

	DefaultChartJobImageRegistry  = "ghcr.io"
	DefaultChartKubectlRepository = "daocloud/kubectl"
	DefaultChartKubectlTag        = "v1.25.3"
	DefaultChartCfsslRepository   = "daocloud/cfssl"
	DefaultChartCfsslTag          = "v1.6.3"
)
