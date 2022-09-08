package helm

import (
	// "fmt"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"

	installv1alpha1 "github.com/daocloud/karmada-operator/pkg/apis/install/v1alpha1"
)

var (
	// All of karmada dependency pod images of kubernates.
	Kubernates = []string{"apiServer", "kubeControllerManager", "etcd"}

	// All of karmada support pod images.
	Karmada = []string{
		"schedulerEstimator", "descheduler",
		"search", "scheduler", "webhook",
		"controllerManager", "agent", "aggregatedApiServer",
	}

	// All of docker.io support pod images.
	DockerIo = []string{
		"cfssl", "kubectl",
	}
)

type InstallMode string

const (
	HostInstallMode      InstallMode = "host"
	AgentInstallMode     InstallMode = "agent"
	ComponentInstallMode InstallMode = "component"
)

type Values struct {
	InstallMode InstallMode                 `yaml:"installMode,omitempty"`
	Cert        *Cert                       `yaml:"certs,omitempty"`
	Components  []installv1alpha1.Component `yaml:"components,omitempty"`
	ETCD        ETCD                        `yaml:"etcd,omitempty"`
	Modules     map[string]Module           `yaml:",inline"`
}

type Cert struct {
	Mode string    `yaml:"mode,omitempty"`
	Auto *AutoCert `yaml:"auto,omitempty"`
}

type AutoCert struct {
	Expiry string   `yaml:"expiry,omitempty"`
	Hosts  []string `yaml:"hosts,omitempty"`
}

type Module struct {
	ReplicaCount *int32             `yaml:"replicaCount,omitempty"`
	Image        *Image             `yaml:"image,omitempty"`
	HostNetwork  *bool              `yaml:"hostNetwork,omitempty"`
	ServiceType  corev1.ServiceType `yaml:"serviceType,omitempty"`
	NodePort     int32              `yaml:"nodePort,omitempty"`
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

func (v *Values) ValuesWithHostInstallMode() ([]byte, error) {
	vc := *v
	vc.InstallMode = HostInstallMode
	vc.Components = nil

	// TODO: remove redundant module.
	return yaml.Marshal(vc)
}

func (v *Values) ValuesWithComponentInstallMode() ([]byte, error) {
	vc := *v
	vc.InstallMode = ComponentInstallMode
	vc.ETCD = ETCD{}
	return yaml.Marshal(vc)
}

func Compose(kd *installv1alpha1.KarmadaDeployment) *Values {
	return Convert_KarmadaDeployment_To_Values(kd)
}

func Convert_KarmadaDeployment_To_Values(kmd *installv1alpha1.KarmadaDeployment) *Values {
	values := &Values{}
	if kmd == nil {
		return values
	}

	// Convert ETCD
	if kmd.Spec.ControlPlane.ETCD != nil {
		etcd := ETCD{
			Internal: Internal{
				StorageType: kmd.Spec.ControlPlane.ETCD.StorageMode,
			},
		}
		if len(kmd.Spec.ControlPlane.ETCD.StorageClass) > 0 || len(kmd.Spec.ControlPlane.ETCD.Size) > 0 {
			etcd.Internal.PVC = PVC{
				StorageClass: kmd.Spec.ControlPlane.ETCD.StorageClass,
				Size:         kmd.Spec.ControlPlane.ETCD.Size,
			}
		}
		values.ETCD = etcd
	}

	modeImages := make(map[string]*Image)
	if kmd.Spec.Images != nil {
		for _, k := range Karmada {
			image := &Image{}
			if len(kmd.Spec.Images.KarmadaRegistry) > 0 {
				image.Registry = kmd.Spec.Images.KarmadaRegistry
			}
			if len(kmd.Spec.Images.KarmadaVersion) > 0 {
				image.Tag = kmd.Spec.Images.KarmadaVersion
			}
			if !image.isEmpty() {
				modeImages[k] = image
			}
		}

		for _, k := range Kubernates {
			image := &Image{}
			if len(kmd.Spec.Images.KubeRegistry) > 0 {
				image.Registry = kmd.Spec.Images.KubeRegistry
			}

			// etcd version is different with kubernetes version.
			if k != "etcd" && len(kmd.Spec.Images.KubeVersion) > 0 {
				image.Tag = kmd.Spec.Images.KubeVersion
			}
			if !image.isEmpty() {
				modeImages[k] = image
			}
		}

		for _, d := range DockerIo {
			image := &Image{}
			if len(kmd.Spec.Images.DockerIoRegistry) > 0 {
				image.Registry = kmd.Spec.Images.DockerIoRegistry
				image.Tag = "latest"
			}

			if !image.isEmpty() {
				modeImages[d] = image
			}
		}
	}

	// TODO: if only have one node on the desctinct cluster. the apiserver
	// replicas must be one. the hostNetwork is conflict.
	// Parse module image and replicas values.
	values.Modules = make(map[string]Module)
	for _, module := range kmd.Spec.ControlPlane.Modules {
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
	if len(kmd.Spec.ControlPlane.Components) > 0 {
		// values.InstallMode = ComponentInstallMode
		values.Components = kmd.Spec.ControlPlane.Components
	}

	// set serviceType to apiservice.
	if len(kmd.Spec.ControlPlane.ServiceType) > 0 {
		apiserver := Module{}
		if module, exist := values.Modules["apiServer"]; exist {
			apiserver = module
		}
		apiserver.ServiceType = kmd.Spec.ControlPlane.ServiceType
		values.Modules["apiServer"] = apiserver
	}

	return values
}

// setDefaultValues set apiserver default values
// set default external ip
// the certificate expires in 10 years
func SetChartDefaultValues(v *Values, releaseNamespace string, externalHosts []string) {
	apiserver := Module{}
	if module, exist := v.Modules["apiServer"]; exist {
		apiserver = module
	}

	defaultHostNetwork := false
	apiserver.HostNetwork = &defaultHostNetwork
	v.Modules["apiServer"] = apiserver

	hosts := []string{
		"kubernetes.default.svc",
		fmt.Sprintf("*.etcd.%s.svc.cluster.local", releaseNamespace),
		fmt.Sprintf("*.%s.svc.cluster.local", releaseNamespace),
		fmt.Sprintf("*.%s.svc", releaseNamespace),
		"localhost",
		"127.0.0.1",
	}
	hosts = append(hosts, externalHosts...)

	v.Cert = &Cert{
		Mode: "auto",
		Auto: &AutoCert{
			Hosts:  hosts,
			Expiry: "87600h",
		},
	}
}
