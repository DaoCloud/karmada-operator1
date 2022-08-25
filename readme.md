# Karmada-Operator

A Karmada Operator based on the K8s Operator model built by DaoCloud, Karmada Operator(Kubernetes Armada operator) simplifies the deployment, operation and maintenance of Karmada.

## TL;DR

Switch to the `root` directory of the repo.

```shell
helm install karmada-operator -n karmada-operator-system --create-namespace --dependency-update ./charts/karmada-operator
```

## Prerequisites

- Kubernetes 1.16+
- helm v3+

## Demo

![screenshot_gif](https://github.com/DaoCloud/karmada-operator/blob/main/docs/gif/demo.gif)

## Installing the Chart <a id="InstallChart"></a>

To install the chart with the release name `karmada-operator` in namespace `karmada-operator-system`:

- local installation

Switch to the `root` directory of the repo.

```shell
helm install karmada-operator -n karmada-operator-system --create-namespace --dependency-update ./charts/karmada-operator
```

> **Tip**: If you need to modify [values.yaml](./charts/karmada-operator/values.yaml),  please refer to [Configuration](#Configuration)

- remote installation

First, add the Karmada-Operator chart repo to your local repository.

```shell
$ helm repo add karmada https://release.daocloud.io/chartrepo/karmada
$ helm repo list
NAME            URL
karmada   https://release.daocloud.io/chartrepo/karmada
```

With the repo added, available charts and versions can be viewed.

```shell
$ helm search repo karmada
NAME                    	CHART VERSION	APP VERSION	DESCRIPTION
karmada/karmada-operator	0.0.5        	0.0.5      	A Helm chart for karmada-operator
```

Install the chart and specify the version to install with the --version argument. Replace `--version=<x.x.x>` with your desired version. such as the above `karmada/karmada-operator` chart version 0.0.5 , so you can use `--version=0.0.5`.

```shell
$ helm --namespace karmada-operator-system upgrade -i karmada-operator karmada/karmada-operator --version=<x.x.x> --create-namespace
Release "karmada-operator" does not exist. Installing it now.
NAME: karmada-operator
LAST DEPLOYED: Thu Aug 11 15:24:01 2022
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

- example
  there is an [example](./examples/karmadaDeployment.yaml) to install a karmada instance on current cluster.

> **Tip**: List all releases using `helm list`


## Create a KarmadaDeployment

After installing karmada-operator, use `cr` to create karmadaDeployment

Create `cr` using below command in the cluster where the karmada-operator install

> **Tip**:
>
> - If no `secretRef` is specified in `cr`, the karmada instance is installed in the cluster where karmada-operator is located
> - If no `namespace` is specified in `cr`, the default install namespace is the same as the karmadaDeployment name

```shell
#deploy KarmadaDeployment demo-kmd in the cluster where the karmada-operator install 
$ kubectl apply -f https://github.com/DaoCloud/karmada-operator/blob/main/examples/karmadaDeployment.yaml
```

The process of deploying karmada may take a few minutes, check the operation of karmada's components

```shell
$ kubectl -n demo-kmd get pod
NAME                                                        READY   STATUS    RESTARTS      AGE
etcd-0                                                      1/1     Running   0             19m
karmada-demo-kmd-aggregated-apiserver-6fd745748-hjbq7       1/1     Running   0             19m
karmada-demo-kmd-apiserver-429bfb7c1-6fkrk                  1/1     Running   0             19m
karmada-demo-kmd-controller-manager-7b8f5699c-mvx8s         1/1     Running   3 (21m ago)   21m
karmada-demo-kmd-controller-manager-7b8f5699c-7fc8c         1/1     Running   2 (19m ago)   19m
karmada-demo-kmd-kube-controller-manager-9b56855fdc-699c5   1/1     Running   0             19m
karmada-demo-kmd-scheduler-685b997cb7-9467w                 1/1     Running   0             19m
karmada-demo-kmd-scheduler-685b997cb7-85c84                 1/1     Running   0             20m
karmada-demo-kmd-webhook-85c84d84c7-g6t9n                   1/1     Running   0             19m
```

ok, karmada installation is complete.

## Create multiple KarmadaDeployments

karmada-operator supports creating multiple karmadaDeployment instances in the same cluster,You can deploy multiple karmada instances multiple times using the `cr` of KarmadaDeployment with different content.

List multiple KarmadaDeployments：

```shell
$ kubectl get kmd
NAME         MODE   PHASE       STATUS   AGE
demo-kmd     Helm   Completed    true    2m42s
demo-kmd-1   Helm   Completed    true    2m57s
$ kubectl get pod -n demo-kmd-1
NAME                                                        	READY   STATUS    RESTARTS      AGE
etcd-0                                                      	1/1     Running   0             14m
karmada-demo-kmd-1-aggregated-apiserver-9b56855fd-gwrmf         1/1     Running   0             14m
karmada-demo-kmd-1-apiserver-6f9bbcb9c-vmxbq                    1/1     Running   0             14m
karmada-demo-kmd-1-controller-manager-5d6777b75-7rmq8           1/1     Running   2 (17m ago)   17m
karmada-demo-kmd-1-controller-manager-5d6777b75-g6xzd           1/1     Running   2 (16m ago)   16m
karmada-demo-kmd-1-kube-controller-manager-76b94fb97c-6lqwv     1/1     Running   0             14m
karmada-demo-kmd-1-scheduler-797b7b787b-klcs4                   1/1     Running   0             14m
karmada-demo-kmd-1-scheduler-797b7b787b-r829t                   1/1     Running   0             15m
karmada-demo-kmd-1-webhook-755d649cb4-wbx5h                     1/1     Running   0             15m
$ kubectl get pod -n demo-kmd
NAME                                                        READY   STATUS    RESTARTS      AGE
etcd-0                                                      1/1     Running   0             19m
karmada-demo-kmd-aggregated-apiserver-6fd745748-hjbq7       1/1     Running   0             19m
karmada-demo-kmd-apiserver-429bfb7c1-6fkrk                  1/1     Running   0             19m
karmada-demo-kmd-controller-manager-7b8f5699c-mvx8s         1/1     Running   3 (21m ago)   21m
karmada-demo-kmd-controller-manager-7b8f5699c-7fc8c         1/1     Running   2 (19m ago)   19m
karmada-demo-kmd-kube-controller-manager-9b56855fdc-699c5   1/1     Running   0             19m
karmada-demo-kmd-scheduler-685b997cb7-9467w                 1/1     Running   0             19m
karmada-demo-kmd-scheduler-685b997cb7-85c84                 1/1     Running   0             20m
karmada-demo-kmd-webhook-85c84d84c7-g6t9n                   1/1     Running   0             19m
```

## Export Karmada KubeConfig

You can get karmada kubeconfig to view karmada instance information

```shell
$ KARMADA_CONFIG=$(kubectl get secret  -n ${your_karmada_operator_namespace} $(kubectl get kmd demo-kmd -o jsonpath='{.status.secretRef.name}') -o jsonpath='{.data.kubeconfig}')
$ echo $KARMADA_CONFIG | base64 -d > karmada-apiserver.config
```

You can export KUBECONFIG for easy login the karmada instance

```shell
export KUBECONFIG=./karmada-apiserver.config
```

We can simply test karmada-apiserver kubeconfig ,such as List namespaces

```shell
$ kubectl --kubeconfig karmada-apiserver.config get ns
NAME              STATUS   AGE
default           Active   124m
demo-kmd          Active   89m
karmada-cluster   Active   89m
kube-node-lease   Active   124m
kube-public       Active   124m
kube-system       Active   124m
```

## Simple Karmada Demo

We can test karmada's multi-cloud capabilities with a simple demo

## Join Clusters

### 1. Use kubectl karmada join cluster to Karmada.

Currently you need to download karmadactl to join cluster

Karmada provides `kubectl-karmada` plug-in download service since v1.2.1. You can choose proper plug-in version which fits your operator system form [karmada release](https://github.com/karmada-io/karmada/releases).

Take v1.2.1 that working with linux-arm64 os as an example:

```shell
$ wget https://github.com/karmada-io/karmada/releases/download/v1.2.1/karmadactl-linux-arm64.tgz

$ tar -zxf karmadactl-linux-arm64.tgz

$ cp karmadactl /usr/local/bin
```

then we join `member1` and `member2` cluster into karmada

`--cluster-kubeconfig` is your members kubeconfig.

`--kubeconfig` is your karmada apiserver kubeconfig

```shell
$ karmadactl --kubeconfig ${your_karmada_apiserver_config} join member1 --cluster-kubeconfig=${your_member1_kubeconfig}
$ karmadactl --kubeconfig ${your_karmada_apiserver_config} join member2 --cluster-kubeconfig=${your_member2_kubeconfig}
```

### 2. Show members of karmada

```shell
$ kubectl  --kubeconfig ${your_karmada_apiserver_config} get clusters
NAME         VERSION    MODE   READY   AGE
member1      v1.21.12   Push   True    2m53s
member2      v1.22.6    Push   True    21s
```

## Propagate a deployment by Karmada

### 1. Create nginx deployment in Karmada

First, create a deployment named `nginx`:

```shell
# create a deployment name nginx
$ kubectl create -f https://github.com/DaoCloud/karmada-operator/blob/main/examples/nginx/deployment.yaml --kubeconfig ${your_karmada_apiserver_config}
```

### 2. Create PropagationPolicy that will propagate nginx to member cluster

Then, we need to create a policy to propagate the deployment to our member cluster.

```shell
# create a PropagationPolicy 
$ kubectl create -f https://github.com/DaoCloud/karmada-operator/blob/main/examples/nginx/propagationpolicy.yaml --kubeconfig ${your_karmada_apiserver_config}
```

### 3. Check the deployment status from Karmada

You can check deployment status from Karmada, don't need to access member cluster:

```shell
$ kubectl get deployment --kubeconfig ${your_karmada_apiserver_config}
NAME    READY   UP-TO-DATE   AVAILABLE   AGE
nginx   2/2     2            2           20s
```

## Configuration<a id="Configuration"></a>

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

## Uninstalling

### Uninstalling the KarmadaDeployment

Deleting karmadaDeployment is a dangerous operation and needs to be deleted carefully. Please delete it according to the following process.

#### Only Delete KarmadaDeployment CR

When you want to remove CR but keep the instance on the environment，you can execute the following command

```shell
# patch disable-cascading-deletion label
$ kubectl label --overwrite kmd demo-kmd karmada.install.io/disable-cascading-deletion="true"
# disable cascade delete KarmadaDeployment
$ kubectl delete kmd demo-kmd -n demo-kmd
```

#### Delete KarmadaDeployment CR and Instance

When you want to remove CR and the instance in your environment，you can execute the following command

```shell
# cascade delete KarmadaDeployment
$ kubectl delete kmd demo-kmd -n demo-kmd
```

### Uninstalling the Karmada-Operator

To uninstall/delete the `karmada-operator` helm release in namespace `karmada-operator-system`:

```shell
helm uninstall karmada-operator -n karmada-operator-system
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

```shell
kubectl delete ns karmada-operator-system
```

