## `kexer`

Kexer (K8s Executor) is an addon apiserver to execute commands in a Kubernetes cluster. It is designed to be used to offload long running streaming operations like `exec`, `cp` from the main `apiserver`. It can also be used as a proxy for main `apiserver` with rest of the operations proxied to the main `apiserver`.

Kexer can also be used as a reverse proxy for clusters configured using a secret with endpoint and service-account token on the cluster that acts a `reverse proxy`.

### Highlights

- [x] Execute commands in a Kubernetes cluster
- [x] kubectl compatible
- [x] Support for `kubectl exec` and `kubectl cp` commands
- [x] Support for `kubectl logs` command
- [x] Support for authentication and authorization delegation to the main apiserver

### Installation

#### Prerequisites

- serving certificate and key for the apiserver

#### Steps

1. Generate a serving certificate and key for the apiserver. The certificate and key should be in `PEM` format. The certificate should be either signed by a CA trusted by the `kube-apiserver` (default) or Public CA or self signed. In case of Public CA or self signed, set the caBundle in the `config/kexer-apiservice.yaml` . The serving certificate and key should be set in the `Secret` object `config/kexer-serving-cert.yaml`.

2. Run the following command to install the addon:

```bash
kubectl apply -f https://raw.githubusercontent.com/Commvault/kexer/master/config
```

### Configuration

The addon can be configured as a `NodePort` or `LoadBalancer` service. The default configuration is `ClusterIP`. A sample configuration for `NodePort` service is available in `sample/node-svc.yaml` file.

### Authentication and Authorization

The addon supports authentication and authorization delegation to the main apiserver. To enable this feature, create a kubeconfig with following endpoint url and use the `client certificate` or the `ServiceAccount` token. A sample kubeconfig file is available in `sample/kubeconfig.yaml` file.

Example: 

```yaml
server:  https://woker-node:node-port/apis/backup.cv.io/v1/namespaces/default/clusters/local/exec
```

### Usage

#### `kubectl exec`

```bash
kubectl exec -it <pod-name> -- <command>
```

#### `kubectl cp`

```bash
kubectl cp <pod-name>:<path> <local-path>
```
### Reverse Proxy

Create a `Secret` object with the following keys. You can use the `sample/cluster-creds-secret.yaml` file as a template.

- `endpointUrl`: The endpoint of the cluster to be proxied
- `token`: The service account token for the cluster to be proxied

The url for the reverse proxy in the kubeconfig is:

```yaml
https://<kexer-host>:<kexer-port>/apis/backup.cv.io/v1/namespaces/<secret-namespace>/clusters/<secret-name>/exec
```
