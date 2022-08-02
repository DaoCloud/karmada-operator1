# Karmada-Operator

A Karmada Operator based on the K8s Operator model built by DaoCloud, Karmada Operator(Kubernetes Armada operator) simplifies the deployment, operation and maintenance of Karmada.

## TL;DR

Switch to the `root` directory of the repo.
```console
helm install karmada-operator -n karmada-operator-system --create-namespace --dependency-update ./charts/karmada-operator
```

## Prerequisites

- Kubernetes 1.16+
- helm v3+

## Installing the Chart

To install the chart with the release name `karmada-operator` in namespace `karmada-operator-system`:

- local installation

Switch to the `root` directory of the repo.
```console
helm install karmada-operator -n karmada-operator-system --create-namespace --dependency-update ./charts/karmada-operator
```

- remote installation

First, add the Karmada-Operator chart repo to your local repository.
```console
$ helm repo add karmada-operator-release https://release.daocloud.io/chartrepo/karmada-operator
$ helm repo list
NAME            URL
karmada-operator-release   https://release.daocloud.io/chartrepo/karmada-operator
```
With the repo added, available charts and versions can be viewed.
```console
helm search repo karmada-operator-release
```
Install the chart and specify the version to install with the --version argument. Replace <x.x.x> with your desired version. Now only support --version=v0.0.1.
```console
helm --namespace karmada-operator-system upgrade -i karmada-operator karmada-operator-release/karmada-operator --version=<x.x.x> --create-namespace
Release "karmada-operator" does not exist. Installing it now.
NAME: karmada
LAST DEPLOYED: Mon May 30 07:19:36 2022
NAMESPACE: karmada-operator-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
NOTES:
Thank you for installing karmada-operator.

Your release is named karmada-operator.

To learn more about the release, try:

  $ helm status karmada-operator -n karmada-operator-system
  $ helm get all karmada-operator -n karmada-operator-system
```

> **Tip**: List all releases using `helm list`

## Uninstalling the Chart
To uninstall/delete the `karmada-operator` helm release in namespace `karmada-operator-system`:

```console
helm uninstall karmada-operator -n karmada-operator-system
```

The command removes all the Kubernetes components associated with the chart and deletes the release.
> **Note**: There are some RBAC resources that are used by the `preJob` that can not be deleted by the `uninstall` command above. You might have to clean them manually with tools like `kubectl`.  You can clean them by commands:

```console
kubectl delete sa/karmada-operator-controller-manager -n karmada-operator-system
kubectl delete clusterRole/karmada-operator
kubectl delete clusterRoleBinding/karmada-operator
kubectl delete ns karmada-operator-system
```

## Demo

![screenshot_gif](https://github.com/DaoCloud/karmada-operator/blob/main/docs/demo.gif)


## Example
### 1. Install controller manager
Edited values.yaml
```YAML
## Default values for charts.
## This is a YAML-formatted file.
## Declare variables to be passed into your templates.

## @param global karmada global config
global:
  ## @param global.imageRegistry Global Docker image registry
  imageRegistry: "release.daocloud.io"
  ## E.g.
  ## imagePullSecrets:
  ##   - myRegistryKeySecretName
  imagePullSecrets: []

## @param commonLabels Add labels to all the deployed resources (sub-charts are not considered). Evaluated as a template
##
commonLabels: {}
## @param commonAnnotations Annotations to add to all deployed objects
##
commonAnnotations: {}
## @param installCRDs define flag whether to install CRD resources
##
installCRDs: true

## operator config
operator:
  ## karmada chart resource
  chartResource:
    ## karmada chart resource repository url
    repoUrl: https://release.daocloud.io/chartrepo/karmada
    ## karmada chart version
    version: 0.0.5
    ## karmada chart name
    name: karmada
  ## @param operator.labels
  labels: {}
  ## @param operator.replicaCount target replicas
  replicaCount: 1
  ## @param operator.podAnnotations
  podAnnotations: {}
  ## @param operator.podLabels
  podLabels: {}
  ## @param image.registry karmada-operator operator image registry
  ## @param image.repository karmada-operator operator image repository
  ## @param image.tag karmada-operator operator image tag (immutable tags are recommended)
  ## @param image.pullPolicy karmada-operator operator image pull policy
  ## @param image.pullSecrets Specify docker-registry secret names as an array
  ##
  image:
    registry: release.daocloud.io
    repository: karmada/karmada-operator
    tag: "v0.0.1"
    ## Specify a imagePullPolicy
    ## Defaults to 'Always' if image tag is 'latest', else set to 'IfNotPresent'
    ##
    pullPolicy: IfNotPresent
    ## Optionally specify an array of imagePullSecrets.
    ## Secrets must be manually created in the namespace.
    ## Example:
    ## pullSecrets:
    ##   - myRegistryKeySecretName
    ##
    pullSecrets: []
  ## @param operator.resources
  resources:
    {}
    # If you do want to specify resources, uncomment the following
    # lines, adjust them as necessary, and remove the curly braces after 'resources:'.
    # limits:
    #   cpu: 100m
    #   memory: 128Mi
    # requests:
    #   cpu: 100m
  #   memory: 128Mi
  ## @param operator.nodeSelector
  nodeSelector: {}
  ## @param operator.affinity
  affinity: {}
  ## @param operator.tolerations
  tolerations: []

```
## Configuration

### Global parameters

| Name                      | Description                                     | Value |
| ------------------------- | ----------------------------------------------- | ----- |
| `global.imageRegistry`    | Global Docker image registry                    | `""`  |
| `global.imagePullSecrets` | Global Docker registry secret names as an array | `[]`  |

### Common parameters

| Name                                  | Description                                                  | Value                                                    |
| ------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------- |
| `controllerManager.labels`            | deployment labels of the karmada-operator-controller-manager | `"app: karmada-operator-controller-manager"`             |
| `controllerManager.replicaCount`      | replicas of the karmada-operator-controller-manager  deployment | `2`                                                      |
| `controllerManager.podAnnotations`    | pod Annotations of the karmada-operator-controller-manager   | `{}`                                                     |
| `controllerManager.podLabels`         | pod Labels of the karmada-operator-controller-manager        | `{}`                                                     |
| `controllerManager.image.registry`    | Image registry of the karmada-operator-controller-manager    | `release.daocloud.io`                                 |
| `controllerManager.image.repository`  | Image repository of the karmada-operator-controller-manager  | `kairship/karmada-operator-controller-manager`           |
| `controllerManager.image.tag`         | Image tag of the karmada-operator-controller-manager         | `"v0.0.1"`                                               |
| `controllerManager.image.pullPolicy`  | Image registry of the karmada-operator-controller-manager    | `"IfNotPresent"`                                         |
| `controllerManager.image.pullSecrets` | Image pullSecrets of the karmada-operator-controller-manager | `[]`                                                     |
| `controllerManager.resources`         | resources of the karmada-operator-controller-manager         | `{}`                                                     |
| `controllerManager.nodeSelector`      | nodeSelector of the karmada-operator-controller-manager      | `{}`                                                     |
| `controllerManager.affinity`          | affinity of the karmada-operator-controller-manager          | {}                                                       |
| `controllerManager.tolerations`       | tolerations of the karmada-operator-controller-manager       | `key: node-role.kubernetes.io/master   operator: Exists` |
| `controllerManager.kubeconfigPath`    | kubeconfig Path of the karmada-operator-controller-manager   | `"/root/.kube"`                                          |
| `controllerManager.localKubeconfig`   | Image registry of the karmada-operator-controller-manager    | `"true"`                                                 |
