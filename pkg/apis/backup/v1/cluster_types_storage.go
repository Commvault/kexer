package v1

import (
	"context"

	"github.com/Commvault/kexer/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

var _ rest.Getter = &Cluster{}

// converts a secret to a cluster object
func (in *Cluster) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {

	var e error
	cluster := &Cluster{}

	namespaeName := request.NamespaceValue(ctx)
	secret, err := utils.GetK8sClient().CoreV1().Secrets(namespaeName).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		//ignore error
		return cluster, e
	}
	cluster.Spec.EndpointUrl = string(secret.Data["endpointUrl"])
	cluster.Spec.SecretToken = string(secret.Data["token"])

	return cluster, e

}
