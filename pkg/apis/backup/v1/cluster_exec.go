/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Commvault/kexer/pkg/kubelet"
	"github.com/Commvault/kexer/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	apiproxy "k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/registry/rest"
	restreg "k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	clientgorest "k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	contextutil "sigs.k8s.io/apiserver-runtime/pkg/util/context"
	"sigs.k8s.io/apiserver-runtime/pkg/util/loopback"
)

var _ resource.ConnectorSubResource = &ClusterExec{}

var _ resource.QueryParameterObject = &ClusterExec{}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterExec struct {
	metav1.TypeMeta `json:",inline"`

	// Stdin if true indicates that stdin is to be redirected for the exec call
	Stdin bool

	// Stdout if true indicates that stdout is to be redirected for the exec call
	Stdout bool

	// Stderr if true indicates that stderr is to be redirected for the exec call
	Stderr bool

	// TTY if true indicates that a tty will be allocated for the exec call
	TTY bool

	// Container in which to execute the command.
	Container string

	// Command is the remote command to execute; argv array; not executed within a shell.
	Command []string

	Path string

	Params *url.Values
}

var _ http.Handler = &execHandler{}

type execHandler struct {
	parentName    string
	isExec        bool
	execOpts      *ClusterExec
	clusterConfig *Cluster
	responder     restreg.Responder
}

func (p *ClusterExec) SubResourceName() string {
	return "exec"
}

func (p ClusterExec) New() runtime.Object {
	return &ClusterExec{}
}

func (p *ClusterExec) Connect(ctx context.Context, id string, options runtime.Object, responder rest.Responder) (http.Handler, error) {
	execOpts, ok := options.(*ClusterExec)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", options)
	}
	parentStorage, ok := contextutil.GetParentStorageGetter(ctx)
	if !ok {
		return nil, fmt.Errorf("no parent storage found")
	}
	parentObj, err := parentStorage.Get(ctx, id, &metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("no such cluster %v", id)
	}
	clusterObj := parentObj.(*Cluster)

	ptokens := strings.Split(execOpts.Path, "/")

	return &execHandler{
		parentName:    id,
		clusterConfig: clusterObj,
		execOpts:      execOpts,
		responder:     responder,
		isExec:        ptokens[len(ptokens)-1] == "exec",
	}, nil

}

type proxyResponseWriter struct {
	http.ResponseWriter
	http.Hijacker
	http.Flusher
	statusCode int
}

func (in *proxyResponseWriter) WriteHeader(statusCode int) {
	in.statusCode = statusCode
	in.ResponseWriter.WriteHeader(statusCode)
}

func newProxyResponseWriter(_writer http.ResponseWriter) *proxyResponseWriter {
	writer := &proxyResponseWriter{ResponseWriter: _writer, statusCode: http.StatusOK}
	writer.Hijacker, _ = _writer.(http.Hijacker)
	writer.Flusher, _ = _writer.(http.Flusher)
	return writer
}

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (fn RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

var _ apiproxy.ErrorResponder = ErrorResponderFunc(nil)

// +k8s:deepcopy-gen=false
type ErrorResponderFunc func(w http.ResponseWriter, req *http.Request, err error)

func (e ErrorResponderFunc) Error(w http.ResponseWriter, req *http.Request, err error) {
	e(w, req, err)
}

func (p *execHandler) ServeHTTP(_writer http.ResponseWriter, request *http.Request) {

	var err error

	writer := newProxyResponseWriter(_writer)
	newReq := request.Clone(request.Context())
	newReq.Header = utilnet.CloneHeader(request.Header)
	newReq.RequestURI = newReq.URL.RequestURI()

	cfg, k8sclient, err := NewConfigFromCluster(request.Context(), p.clusterConfig)
	if err != nil {
		responsewriters.InternalError(writer, request, fmt.Errorf("invalid transport"))
		return
	}

	rt, err := clientgorest.TransportFor(cfg)
	if err != nil {
		responsewriters.InternalError(writer, request, fmt.Errorf("invalid transport"))
		return
	}

	var location *url.URL
	var kubeletcfg kubelet.KubeletClientConfig

	if !p.isExec {

		u, _ := url.Parse(cfg.Host)
		location = &url.URL{
			Scheme:   "https",
			Host:     u.Host,
			Path:     p.execOpts.Path,
			RawQuery: p.execOpts.Params.Encode(),
		}
	} else {
		kubeletcfg = kubelet.KubeletClientConfig{
			Port:         10250,
			ReadOnlyPort: 10255,
			PreferredAddressTypes: []string{
				// internal, preferring DNS if reported
				string(corev1.NodeInternalDNS),
				string(corev1.NodeInternalIP),

				// --override-hostname
				string(corev1.NodeHostName),

				// external, preferring DNS if reported
				string(corev1.NodeExternalDNS),
				string(corev1.NodeExternalIP),
			},
			BearerToken: cfg.BearerToken,
			EnableHTTPS: true,
			HTTPTimeout: time.Duration(5) * time.Second,
			TLSClientConfig: clientgorest.TLSClientConfig{
				CertData: cfg.CAData,
				KeyData:  cfg.KeyData,
			},
		}

		nodeConnGetter, _ := kubelet.NewNodeConnectionInfoGetter(
			func(name string) (*corev1.Node, error) {
				return k8sclient.CoreV1().Nodes().Get(request.Context(), name, metav1.GetOptions{})
			}, kubeletcfg)

		location, _, err = ExecLocation(k8sclient.CoreV1(), nodeConnGetter, request.Context(), p.parentName, p.execOpts)
		if err != nil {
			responsewriters.InternalError(writer, request, fmt.Errorf("invalid location"))
			return
		}
	}
	h, _, _ := net.SplitHostPort(location.Host)
	newReq.Host = h
	newReq.URL = location

	proxy := apiproxy.NewUpgradeAwareHandler(
		location,
		rt,
		false,
		false,
		nil)

	if p.isExec {

		transportCfg := kubeletcfg.TransportConfig()
		tlsConfig, err := transport.TLSConfigFor(transportCfg)
		if err != nil {
			responsewriters.InternalError(writer, request, fmt.Errorf("invalid tls config"))
			return
		}
		upgrader, err := transport.HTTPWrappersForConfig(transportCfg, apiproxy.MirrorRequest)
		if err != nil {
			responsewriters.InternalError(writer, request, fmt.Errorf("invalid upgrade request"))
			return
		}
		upgrading := utilnet.SetOldTransportDefaults(&http.Transport{
			TLSClientConfig: tlsConfig,
			DialContext:     cfg.Dial,
		})
		proxy.UpgradeTransport = apiproxy.NewUpgradeRequestRoundTripper(
			upgrading,
			RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				newReq := utilnet.CloneRequest(req)
				return upgrader.RoundTrip(newReq)
			}))
	}

	const defaultFlushInterval = 200 * time.Millisecond

	proxy.Transport = rt
	proxy.FlushInterval = defaultFlushInterval
	proxy.Responder = ErrorResponderFunc(func(w http.ResponseWriter, req *http.Request, err error) {
		p.responder.Error(err)
	})

	proxy.ServeHTTP(writer, newReq)

}

func (p *ClusterExec) NewConnectOptions() (runtime.Object, bool, string) {
	return &ClusterExec{}, true, "path"
}

func (p *ClusterExec) ConnectMethods() []string {
	return []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
}

func (in *ClusterExec) ConvertFromUrlValues(values *url.Values) error {

	in.Stderr = values.Get("stderr") == "true"
	in.Stdin = values.Get("stdin") == "true"
	in.Stdout = values.Get("stdout") == "true"
	in.TTY = values.Get("tty") == "true"
	in.Container = values.Get("container")
	in.Command = (*values)["command"]
	in.Path = values.Get("path")
	in.Params = values

	return nil
}

// ExecLocation returns the exec URL for a pod container. If opts.Container is blank
// and only one container is present in the pod, that container is used.
func ExecLocation(
	getter corev1client.CoreV1Interface,
	connInfo kubelet.ConnectionInfoGetter,
	ctx context.Context,
	name string,
	opts *ClusterExec,
) (*url.URL, http.RoundTripper, error) {

	return streamLocation(getter, connInfo, ctx, name, opts, opts.Container, "exec")

}

func streamLocation(
	getter corev1client.CoreV1Interface,
	connInfo kubelet.ConnectionInfoGetter,
	ctx context.Context,
	name string,
	opts runtime.Object,
	container,
	path string,
) (*url.URL, http.RoundTripper, error) {

	execOpts, _ := opts.(*ClusterExec)
	ptokens := strings.Split(execOpts.Path, "/")
	if len(ptokens) < 5 {
		return nil, nil, errors.NewBadRequest("invalid request")

	}

	podns := ptokens[4]
	podname := ptokens[6]

	pod, err := getter.Pods(podns).Get(ctx, podname, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	if container == "" {
		switch len(pod.Spec.Containers) {
		case 1:
			container = pod.Spec.Containers[0].Name
		case 0:
			return nil, nil, errors.NewBadRequest(fmt.Sprintf("a container name must be specified for pod %s", name))
		default:
			containerNames := getContainerNames(pod.Spec.Containers)
			initContainerNames := getContainerNames(pod.Spec.InitContainers)
			err := fmt.Sprintf("a container name must be specified for pod %s, choose one of: [%s]", name, containerNames)
			if len(initContainerNames) > 0 {
				err += fmt.Sprintf(" or one of the init containers: [%s]", initContainerNames)
			}
			return nil, nil, errors.NewBadRequest(err)
		}
	} else {
		if !podHasContainerWithName(pod, container) {
			return nil, nil, errors.NewBadRequest(fmt.Sprintf("container %s is not valid for pod %s", container, name))
		}
	}

	nodeName := types.NodeName(pod.Spec.NodeName)
	if len(nodeName) == 0 {
		// If pod has not been assigned a host, return an empty location
		return nil, nil, errors.NewBadRequest(fmt.Sprintf("pod %s does not have a host assigned", name))
	}
	nodeInfo, err := connInfo.GetConnectionInfo(ctx, nodeName)
	if err != nil {
		return nil, nil, err
	}
	params := url.Values{}
	if err := streamParams(params, opts); err != nil {
		return nil, nil, err
	}

	loc := &url.URL{
		Scheme:   nodeInfo.Scheme,
		Host:     net.JoinHostPort(nodeInfo.Hostname, nodeInfo.Port),
		Path:     fmt.Sprintf("/%s/%s/%s/%s", path, pod.Namespace, pod.Name, container),
		RawQuery: params.Encode(),
	}
	return loc, nodeInfo.Transport, nil
}

// getContainerNames returns a formatted string containing the container names
func getContainerNames(containers []corev1.Container) string {
	names := []string{}
	for _, c := range containers {
		names = append(names, c.Name)
	}
	return strings.Join(names, " ")
}

func podHasContainerWithName(pod *corev1.Pod, containerName string) bool {
	var hasContainer = false
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName {
			hasContainer = true
		}
	}
	return hasContainer
}

func streamParams(params url.Values, opts runtime.Object) error {
	switch opts := opts.(type) {
	case *ClusterExec:
		if opts.Stdin {
			params.Add(corev1.ExecStdinParam, "1")
		}
		if opts.Stdout {
			params.Add(corev1.ExecStdoutParam, "1")
		}
		if opts.Stderr {
			params.Add(corev1.ExecStderrParam, "1")
		}
		if opts.TTY {
			params.Add(corev1.ExecTTYParam, "1")
		}
		for _, c := range opts.Command {
			params.Add("command", c)
		}
	default:
		return fmt.Errorf("unknown object for streaming: %v", opts)
	}
	return nil
}

func NewConfigFromCluster(ctx context.Context, c *Cluster) (*clientgorest.Config, kubernetes.Interface, error) {

	cfg := &clientgorest.Config{
		Timeout: time.Second * 40,
	}

	if c.Spec.EndpointUrl == "" && c.Spec.SecretToken == "" {
		cfg := loopback.GetLoopbackMasterClientConfig()
		if cfg == nil {
			return nil, nil, fmt.Errorf("invalid client config")
		}

		return cfg, utils.GetK8sClient(), nil
	}

	cfg.Host = c.Spec.EndpointUrl
	cfg.TLSClientConfig = clientgorest.TLSClientConfig{Insecure: true}
	cfg.BearerToken = c.Spec.SecretToken

	k8sclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	return cfg, k8sclient, nil
}
