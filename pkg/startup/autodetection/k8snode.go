// Copyright (c) 2016,2020 Tigera, Inc. All rights reserved.
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
package autodetection

import (
	v1 "k8s.io/api/core/v1"

	cnet "github.com/projectcalico/libcalico-go/lib/net"
)

// GetKubernetesInternalIPAddresses returns the internal IP addresses configured on the node status.
func GetKubernetesInternalIPAddresses(version int, node *v1.Node) ([]cnet.IPNet, error) {
	var cidrs []cnet.IPNet
	var err error
	for _, nodeAddr := range node.Status.Addresses {
		if nodeAddr.Type == v1.NodeInternalIP {
			if _, cidr, cerr := cnet.ParseCIDROrIP(nodeAddr.Address); cerr != nil {
				err = cerr
			} else if cidr.Version() == version {
				cidrs = append(cidrs, *cidr)
			}
		}
	}

	return cidrs, err
}
