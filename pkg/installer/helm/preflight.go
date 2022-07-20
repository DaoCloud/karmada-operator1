package helm

import helm "github.com/daocloud/karmada-operator/pkg/helm"

type Preflight struct {
	chartPath  string
	helmClient helm.Client
}

func (p *Preflight) FetchChart() {
}

func (p *Preflight) ComposeValues() {
}
