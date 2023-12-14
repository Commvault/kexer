package utils

import (
	"k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/apiserver-runtime/pkg/util/loopback"
)

var kubeClient kubernetes.Interface

func GetK8sClient() kubernetes.Interface {
	return kubeClient
}

func InitK8sClient(ctx server.PostStartHookContext) error {

	var err error
	cfg := loopback.GetLoopbackMasterClientConfig()
	if cfg == nil {
		return err
	}
	copiedCfg := restclient.CopyConfig(cfg)
	copiedCfg.RateLimiter = nil
	kubeClient, err = kubernetes.NewForConfig(copiedCfg)
	if err != nil {
		return err
	}

	return err

}
