package helm

import (
	"strings"

	"gopkg.in/yaml.v2"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
)

var (
	// All of karmada dependency pod images of kubernates.
	Kubernates = []string{"apiServer", "kubeControllerManager", "etcd"}

	// All of karmada support pod images.
	Karmada = []string{
		"schedulerEstimator", "descheduler",
		"search", "scheduler", "webhook",
		"controllerManager", "agent", "aggregatedApiServer"}
)

const (
	HostInstallMode      = "host"
	AgentInstallMode     = "agent"
	ComponentInstallMode = "component"
)

type Values struct {
	// InstallMode string                      `yaml:"installMode,omitempty"`
	Components []installv1alpha1.Component `yaml:"components,omitempty"`
	ETCD       ETCD                        `yaml:"etcd,omitempty"`
	Modules    map[string]Module           `yaml:",inline"`
}

type Module struct {
	ReplicaCount *int32 `yaml:"replicaCount,omitempty"`
	Image        *Image `yaml:"image,omitempty"`
}

type Image struct {
	Registry   string `yaml:"registry,omitempty"`
	Repository string `yaml:"repository,omitempty"`
	Tag        string `yaml:"tag,omitempty"`
}

type ETCD struct {
	Mode     string   `yaml:"-,omitempty"`
	Internal Internal `yaml:"internal,omitempty"`
}

type Internal struct {
	Module      `yaml:",inline"`
	StorageType string `yaml:"storageType,omitempty"`
	PVC         PVC    `yaml:"pvc,omitempty"`
}

type PVC struct {
	StorageClass string `yaml:"storageClass,omitempty"`
	// TOOD:
	Size string `yaml:"size,omitempty"`
}

func (i *Image) isEmpty() bool {
	return len(i.Registry) == 0 && len(i.Repository) == 0 && len(i.Tag) == 0
}

func ComposeValues(kd *installv1alpha1.KarmadaDeployment) ([]byte, error) {
	values := Convert_KarmadaDeployment_To_Values(kd)
	return yaml.Marshal(values)
}

func Convert_KarmadaDeployment_To_Values(kd *installv1alpha1.KarmadaDeployment) *Values {
	values := &Values{}
	if kd == nil {
		return values
	}

	// Convert ETCD
	if kd.Spec.ControlPlane.ETCD != nil {
		etcd := ETCD{
			Internal: Internal{
				StorageType: kd.Spec.ControlPlane.ETCD.StorageMode,
			},
		}

		if len(kd.Spec.ControlPlane.ETCD.StorageClass) > 0 || len(kd.Spec.ControlPlane.ETCD.Size) > 0 {
			etcd.Internal.PVC = PVC{
				StorageClass: kd.Spec.ControlPlane.ETCD.StorageClass,
				Size:         kd.Spec.ControlPlane.ETCD.Size,
			}
		}
		values.ETCD = etcd
	}

	modeImages := make(map[string]*Image)
	if kd.Spec.Images != nil {
		for _, k := range Karmada {
			image := &Image{}
			if len(kd.Spec.Images.KarmadaRegistry) > 0 {
				image.Registry = kd.Spec.Images.KarmadaRegistry
			}
			if len(kd.Spec.Images.KarmadaVersion) > 0 {
				image.Tag = kd.Spec.Images.KarmadaVersion
			}
			if !image.isEmpty() {
				modeImages[k] = image
			}
		}

		for _, k := range Kubernates {
			image := &Image{}
			if len(kd.Spec.Images.KubeResgistry) > 0 {
				image.Registry = kd.Spec.Images.KubeResgistry
			}

			// etcd version is different with kubernetes version.
			if k != "etcd" && len(kd.Spec.Images.KubeVersion) > 0 {
				image.Tag = kd.Spec.Images.KubeVersion
			}
			if !image.isEmpty() {
				modeImages[k] = image
			}
		}
	}

	// TODO: if only have one node on the desctinct cluster. the apiserver
	// replicas must be one. the hostNetwork is conflict.
	// Parse module image and replicas values.
	values.Modules = make(map[string]Module)
	for _, module := range kd.Spec.ControlPlane.Modules {
		name := string(module.Name)
		// TODO: if component is disabled, skip the loop.

		m := Module{}
		if module.Replicas != nil {
			m.ReplicaCount = module.Replicas
		}

		var registry, repository, tag string
		if len(module.Image) > 0 {
			image := module.Image
			i := strings.LastIndex(image, ":")
			if i > 0 {
				tag = image[i+1:]
				image = image[:i]
			}
			if strings.Contains(image, "/") {
				registry, repository, _ = strings.Cut(image, "/")
			} else {
				repository = image
			}
		}

		modeImages[name] = &Image{
			Registry:   registry,
			Repository: repository,
			Tag:        tag,
		}

		values.Modules[name] = m
	}

	for k, image := range modeImages {
		if k == "etcd" {
			values.ETCD.Internal.Image = image
			continue
		}

		if m, exist := values.Modules[k]; exist {
			m.Image = image
			values.Modules[k] = m
		} else {
			values.Modules[k] = Module{
				Image: image,
			}
		}
	}

	// TODO: it's not work.
	if len(kd.Spec.ControlPlane.Components) > 0 {
		// values.InstallMode = ComponentInstallMode
		values.Components = kd.Spec.ControlPlane.Components
	}

	return values
}
