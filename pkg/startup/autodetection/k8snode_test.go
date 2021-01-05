// Copyright (c) 2016 Tigera, Inc. All rights reserved.

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
package autodetection_test

import (
	v1 "k8s.io/api/core/v1"

	"github.com/projectcalico/node/pkg/startup/autodetection"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kubernetes nodes tests", func() {

	It("should extract IP addresses", func() {
		node := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{{
					Type:    v1.NodeInternalDNS,
					Address: "hello",
				}, {
					Type:    v1.NodeExternalIP,
					Address: "1.2.3.4",
				}, {
					Type:    v1.NodeInternalIP,
					Address: "1.2.3.40",
				}, {
					Type:    v1.NodeExternalIP,
					Address: "1.2.30.4",
				}, {
					Type:    v1.NodeInternalIP,
					Address: "3333::1122",
				}, {
					Type:    v1.NodeInternalIP,
					Address: "1.20.3.4",
				}},
			},
		}

		By("Checking IPv4 addresses extracted")
		cidrs, err := autodetection.GetKubernetesInternalIPAddresses(4, node)
		Expect(err).NotTo(HaveOccurred())
		Expect(cidrs).To(HaveLen(2))
		Expect(cidrs[0].String()).To(Equal("1.2.3.40"))
		Expect(cidrs[1].String()).To(Equal("1.20.3.4"))

		By("Checking IPv6 address extracted")
		cidrs, err = autodetection.GetKubernetesInternalIPAddresses(6, node)
		Expect(err).NotTo(HaveOccurred())
		Expect(cidrs).To(HaveLen(1))
		Expect(cidrs[0].String()).To(Equal("3333::1122"))
	})

	It("should handle a bad IP value", func() {
		node := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{{
					Type:    v1.NodeInternalIP,
					Address: "hello",
				}},
			},
		}

		By("Checking IPv4 address not extracted and error raised")
		cidrs, err := autodetection.GetKubernetesInternalIPAddresses(4, node)
		Expect(err).To(HaveOccurred())
		Expect(cidrs).To(BeNil())
	})

	It("should handle a bad IP value and a good one", func() {
		node := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{{
					Type:    v1.NodeInternalIP,
					Address: "hello",
				}, {
					Type:    v1.NodeInternalIP,
					Address: "1.2.3.4",
				}},
			},
		}

		By("Checking IPv4 addresses extracted and error raised")
		cidrs, err := autodetection.GetKubernetesInternalIPAddresses(4, node)
		Expect(err).To(HaveOccurred())
		Expect(cidrs).To(HaveLen(1))
		Expect(cidrs[0].String()).To(Equal("1.2.3.4"))
	})

	It("should handle an empty status", func() {
		node := &v1.Node{}

		By("Checking IPv4 addresses not extracted")
		cidrs, err := autodetection.GetKubernetesInternalIPAddresses(4, node)
		Expect(err).NotTo(HaveOccurred())
		Expect(cidrs).To(BeNil())
	})
})
