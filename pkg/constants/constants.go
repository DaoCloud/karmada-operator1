package constants

const (
	// Default karmada chart install system.
	DefaultKarmadaVersion = "v1.2.0"
	// Default karmada chart install system.
	KarmadaNamespace = "karmada-system"
	// karmada apiserver default port.
	KarmadaAPIServerNodePort = 32443
	// karmada chart download dir.
	ChartBasePath = "/var/run/karmada-operator"
	// KubeconfigBasePath = "/var/run/karmada-operator/kubeconfig".
	KubeconfigBasePath = ChartBasePath + "/config"
)
