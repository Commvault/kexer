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

package main

import (
	"net/http"

	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"
	"sigs.k8s.io/apiserver-runtime/pkg/builder"

	// +kubebuilder:scaffold:resource-imports
	execv1 "github.com/Commvault/kexer/pkg/apis/backup/v1"
	"github.com/Commvault/kexer/pkg/utils"
)

func main() {
	err := builder.APIServer.
		WithResource(&execv1.Cluster{}). // namespaced resource
		WithLocalDebugExtension().
		WithoutEtcd().
		ExposeLoopbackMasterClientConfig().
		ExposeLoopbackAuthorizer().
		WithConfigFns(func(config *server.RecommendedConfig) *server.RecommendedConfig {
			config.LongRunningFunc = func(r *http.Request, requestInfo *request.RequestInfo) bool {
				if requestInfo.Subresource == "exec" {
					return true
				}
				return false
			}
			return config
		}).
		WithPostStartHook("init-master-loopback-client", utils.InitK8sClient).
		Execute()

	if err != nil {
		klog.Fatal(err)
	}

}
