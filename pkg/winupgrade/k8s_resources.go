// Copyright (c) 2021 Tigera, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package winupgrade

import (
	"context"
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// k8snode holds a collection of helper functions for Kubernetes node.
type k8snode string

// Add / remove node annotations to node. Perform Get/Check/Update so that it always working on latest version.
// If node labels has been set already, do nothing.
func (n k8snode) addRemoveNodeAnnotations(k8sClientset *kubernetes.Clientset,
	toAdd map[string]string,
	toRemove []string) error {
	nodeName := string(n)
	return wait.PollImmediate(3*time.Second, 1*time.Minute, func() (bool, error) {
		node, err := k8sClientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		needUpdate := false
		for k, v := range toAdd {
			if currentVal, ok := node.Annotations[k]; ok && currentVal == v {
				continue
			}
			node.Annotations[k] = v
			needUpdate = true
		}

		for _, k := range toRemove {
			if _, ok := node.Annotations[k]; ok {
				delete(node.Annotations, k)
				needUpdate = true
			}
		}

		if needUpdate {
			_, err := k8sClientset.CoreV1().Nodes().UpdateStatus(context.Background(), node, metav1.UpdateOptions{})
			if err == nil {
				return true, nil
			}
			if !apierrs.IsConflict(err) {
				return false, err
			}

			// Retry on update conflicts.
			return false, nil
		}

		// no update needed
		return true, nil
	})
}

// daemonset holds a collection of helper functions for Kubernetes daemonset.
type daemonset string

// Get spec of a container from a daemonset.
func (d daemonset) getContainerSpec(cs kubernetes.Interface, namespace, containerName string) (*v1.Container, error) {
	ds, err := cs.AppsV1().DaemonSets(namespace).Get(context.Background(), string(d), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	for _, c := range ds.Spec.Template.Spec.InitContainers {
		if c.Name == containerName {
			return &c, nil
		}
	}

	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == containerName {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("No container with name %s found in daemonset", containerName)
}

// Get container image from a container spec.
func (d daemonset) getContainerImage(cs kubernetes.Interface, namespace, containerName string) (string, error) {
	container, err := d.getContainerSpec(cs, namespace, containerName)
	if err != nil {
		return "", err
	}

	return container.Image, nil
}
