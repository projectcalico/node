// Copyright (c) 2020 Tigera, Inc. All rights reserved.
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
	"github.com/projectcalico/libcalico-go/lib/net"
	kapiv1 "k8s.io/api/core/v1"
)

func nodeAddresses(node *kapiv1.Node, addrType kapiv1.NodeAddressType) []string {
	ret := make([]string, 0, 1)
	for _, addr := range node.Status.Addresses {
		if addr.Type == addrType {
			ret = append(ret, addr.Address)
		}
	}

	return ret
}

// K8sNodeInternalIPs returns the internal IPs of the given version found in the k8s Node resource
func K8sNodeInternalIPs(node *kapiv1.Node, version int) []*net.IPNet {
	addrs := nodeAddresses(node, kapiv1.NodeInternalIP)
	if len(addrs) == 0 {
		return nil
	}

	ret := make([]*net.IPNet, 0, len(addrs))
	for _, a := range addrs {
		ip, ipnet, err := net.ParseCIDROrIP(a)
		if err == nil && ip.Version() == version {
			ret = append(ret, ipnet)
		}
	}

	return ret
}
