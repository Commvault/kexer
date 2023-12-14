/*
Copyright YEAR The Kubernetes Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
)

func init() {
	for _, fn := range []func(*runtime.Scheme) error{
		AddToScheme,
		corev1.AddToScheme,
	} {
		if err := fn(runtime.NewScheme()); err != nil {
			klog.Fatalf("failed registering core api types")
		}
	}
}

var AddToScheme = func(scheme *runtime.Scheme) error {
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{
		Group:   "backup.cv.io",
		Version: "v1",
	})
	// +kubebuilder:scaffold:install
	scheme.AddKnownTypes(schema.GroupVersion{
		Group:   "backup.cv.io",
		Version: "v1",
	}, &Cluster{}, &ClusterList{})
	return nil
}
