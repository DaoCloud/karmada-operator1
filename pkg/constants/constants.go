package constants

const (
	// Default karmada chart install system.
	KarmadaNamespace = "karmada-system"
	// karmada chart download dir.
	ChartBasePath = "/var/run/karmada-operator"
	// KubeconfigBasePath = "/var/run/karmada-operator/kubeconfig".
	KubeconfigBasePath = ChartBasePath + "/config"
)
