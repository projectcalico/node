// Copyright (c) 2018 Tigera, Inc. All rights reserved.

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

package allocateip

import (
	"context"
	"errors"
	"fmt"
	gnet "net"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/projectcalico/libcalico-go/lib/apiconfig"
	api "github.com/projectcalico/libcalico-go/lib/apis/v3"
	"github.com/projectcalico/libcalico-go/lib/backend"
	client "github.com/projectcalico/libcalico-go/lib/clientv3"
	"github.com/projectcalico/libcalico-go/lib/logutils"
	"github.com/projectcalico/libcalico-go/lib/net"
	"github.com/projectcalico/libcalico-go/lib/options"
	"github.com/projectcalico/libcalico-go/lib/ipam"
)

func allocateIPDescribe(description string, tunnelType []string, body func(tunnelType string)) bool {
	for _, tt := range tunnelType {
		switch tt {
		case "ipip":
			Describe(fmt.Sprintf("%s (ipip)", description),
				func() {
					body(tt)
				})
		case "vxlan":
			Describe(fmt.Sprintf("%s (vxlan)", description),
				func() {
					body(tt)
				})
		default:
			panic(errors.New(fmt.Sprintf("Unknown tunnelType, %s", tt)))
		}
	}

	return true
}

func setTunnelAddressForNode(tunnelType string, n *api.Node, addr string) {
	if tunnelType == "ipip" {
		n.Spec.BGP.IPv4IPIPTunnelAddr = addr
	} else if tunnelType == "vxlan" {
		n.Spec.IPv4VXLANTunnelAddr = addr
	} else {
		panic(errors.New(fmt.Sprintf("Unknown tunnelType, %s", tunnelType)))
	}
}

func checkTunnelAddressForNode(tunnelType string, n *api.Node, addr string) {
	if tunnelType == "ipip" {
		Expect(n.Spec.BGP.IPv4IPIPTunnelAddr).To(Equal(addr))
	} else if tunnelType == "vxlan" {
		Expect(n.Spec.IPv4VXLANTunnelAddr).To(Equal(addr))
	} else {
		panic(errors.New(fmt.Sprintf("Unknown tunnelType, %s", tunnelType)))
	}
}

func checkIPAMAttr(tunnelType string, attr map[string]string) {
	if tunnelType == "ipip" {
		Expect(attr[ipam.AttributeType]).To(Equal(ipipIPAMAttrString))
	} else if tunnelType == "vxlan" {
		Expect(attr[ipam.AttributeType]).To(Equal(vxlanIPAMAttrString))
	} else {
		panic(errors.New(fmt.Sprintf("Unknown tunnelType, %s", tunnelType)))
	}
}

var _ = allocateIPDescribe("ensureHostTunnelAddress", []string{"ipip", "vxlan"}, func(tunnelType string) {
	log.SetOutput(os.Stdout)
	// Set log formatting.
	log.SetFormatter(&logutils.Formatter{})
	// Install a hook that adds file and line number information.
	log.AddHook(&logutils.ContextHook{})

	ctx := context.Background()
	cfg, _ := apiconfig.LoadClientConfigFromEnvironment()

	isVxlan := (tunnelType == "vxlan")

	var c client.Interface
	BeforeEach(func() {
		// Clear out datastore
		be, err := backend.NewClient(*cfg)
		Expect(err).ToNot(HaveOccurred())
		be.Clean()

		//create client.
		c, _ = client.New(*cfg)

		//create IPPool which has only two ips available.
		_, err = c.IPPools().Create(ctx, makeIPv4Pool("172.16.0.0/31", 31), options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Pre-allocate a WEP ip on 172.16.0.0. This will force tunnel address to use 172.16.0.1
		handle := "myhandle"
		wepIp := gnet.IP{172, 16, 0, 0}
		err = c.IPAM().AssignIP(ctx, ipam.AssignIPArgs{
			IP:       net.IP{wepIp},
			Hostname: "test.node",
			HandleID: &handle,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should add tunnel address to node", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, ip4net, _ := net.ParseCIDR("172.16.0.0/31")
		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")
	})

	It("should add tunnel address to node without BGP Spec", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		node.Spec.BGP = nil

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, ip4net, _ := net.ParseCIDR("172.16.0.0/31")
		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")
	})

	It("should assign new tunnel address to node on ippool update", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		setTunnelAddressForNode(tunnelType, node, "172.16.10.10")

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, ip4net, _ := net.ParseCIDR("172.16.0.0/31")
		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")
	})

	It("should assign new tunnel address to node on pre-allocated address", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		setTunnelAddressForNode(tunnelType, node, "172.16.0.0")

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, ip4net, _ := net.ParseCIDR("172.16.0.0/31")
		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")

		// Verify 172.16.0.0 has not been released.
		_, err = c.IPAM().GetAssignmentAttributes(ctx, net.IP{IP: gnet.ParseIP("172.16.0.0")})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should assign new tunnel address to node on unassigned address, and do nothing if node restart", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		setTunnelAddressForNode(tunnelType, node, "172.16.0.1")

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Verify 172.16.0.1 is not properly assigned to tunnel address.
		_, err = c.IPAM().GetAssignmentAttributes(ctx, net.IP{IP: gnet.ParseIP("172.16.0.1")})
		Expect(err).To(HaveOccurred())

		_, ip4net, _ := net.ParseCIDR("172.16.0.0/31")
		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")

		// 172.16.0.1 is properly assigned to tunnel address.
		attr, err := c.IPAM().GetAssignmentAttributes(ctx, net.IP{IP: gnet.ParseIP("172.16.0.1")})
		Expect(err).NotTo(HaveOccurred())
		checkIPAMAttr(tunnelType, attr)

		// Now we have a wep IP allocated at 172.16.0.0 and tunnel ip allocated at 172.16.0.1.
		// Release wep IP and call ensureHostTunnelAddress again. Tunnel ip should not be changed.
		err = c.IPAM().ReleaseByHandle(ctx, "myhandle")
		Expect(err).NotTo(HaveOccurred())

		ensureHostTunnelAddress(ctx, c, node.Name, []net.IPNet{*ip4net}, isVxlan)
		n, err = c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "172.16.0.1")
	})
})

var _ = allocateIPDescribe("removeHostTunnelAddress", []string{"ipip", "vxlan"}, func(tunnelType string) {
	log.SetOutput(os.Stdout)
	// Set log formatting.
	log.SetFormatter(&logutils.Formatter{})
	// Install a hook that adds file and line number information.
	log.AddHook(&logutils.ContextHook{})

	ctx := context.Background()
	cfg, _ := apiconfig.LoadClientConfigFromEnvironment()

	isVxlan := (tunnelType == "vxlan")

	var c client.Interface
	BeforeEach(func() {
		// Clear out datastore
		be, err := backend.NewClient(*cfg)
		Expect(err).ToNot(HaveOccurred())
		be.Clean()

		//create client and IPPool
		c, _ = client.New(*cfg)
		c.IPPools().Create(ctx, makeIPv4Pool("172.16.0.0/24", 26), options.SetOptions{})
	})

	It("should remove tunnel address from node", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		setTunnelAddressForNode(tunnelType, node, "172.16.0.5")

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		removeHostTunnelAddr(ctx, c, node.Name, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		checkTunnelAddressForNode(tunnelType, n, "")
	})

	It("should not panic on node without BGP Spec", func() {
		node := makeNode("192.168.0.1/24", "fdff:ffff:ffff:ffff:ffff::/80")
		node.Name = "test.node"
		node.Spec.BGP = nil

		_, err := c.Nodes().Create(ctx, node, options.SetOptions{})
		Expect(err).NotTo(HaveOccurred())

		removeHostTunnelAddr(ctx, c, node.Name, isVxlan)
		n, err := c.Nodes().Get(ctx, node.Name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(n.Spec.BGP).To(BeNil())
	})
})

var _ = Describe("determineEnabledPoolCIDRs", func() {
	log.SetOutput(os.Stdout)
	// Set log formatting.
	log.SetFormatter(&logutils.Formatter{})
	// Install a hook that adds file and line number information.
	log.AddHook(&logutils.ContextHook{})

	Context("IPIP tests", func() {
		It("should match ip-pool-1 but not ip-pool-2", func() {
			// Mock out the node and ip pools
			n := api.Node{ObjectMeta: metav1.ObjectMeta{Name: "bee-node", Labels: map[string]string{"foo": "bar"}}}
			pl := api.IPPoolList{
				Items: []api.IPPool{
					api.IPPool{
						ObjectMeta: metav1.ObjectMeta{Name: "ip-pool-1"},
						Spec: api.IPPoolSpec{
							Disabled:     false,
							CIDR:         "172.0.0.0/9",
							NodeSelector: `foo == "bar"`,
							IPIPMode:     api.IPIPModeAlways,
						},
					}, api.IPPool{
						ObjectMeta: metav1.ObjectMeta{Name: "ip-pool-2"},
						Spec: api.IPPoolSpec{
							Disabled:     false,
							CIDR:         "172.128.0.0/9",
							NodeSelector: `foo != "bar"`,
							IPIPMode:     api.IPIPModeAlways,
						},
					}}}

			// Execute and test assertions.
			cidrs := determineEnabledPoolCIDRs(n, pl, false)
			_, cidr1, _ := net.ParseCIDR("172.0.0.1/9")
			_, cidr2, _ := net.ParseCIDR("172.128.0.1/9")
			Expect(cidrs).To(ContainElement(*cidr1))
			Expect(cidrs).ToNot(ContainElement(*cidr2))
		})
	})

	Context("VXLAN tests", func() {
		It("should match ip-pool-1 but not ip-pool-2", func() {
			// Mock out the node and ip pools
			n := api.Node{ObjectMeta: metav1.ObjectMeta{Name: "bee-node", Labels: map[string]string{"foo": "bar"}}}
			pl := api.IPPoolList{
				Items: []api.IPPool{
					api.IPPool{
						ObjectMeta: metav1.ObjectMeta{Name: "ip-pool-1"},
						Spec: api.IPPoolSpec{
							Disabled:     false,
							CIDR:         "172.0.0.0/9",
							NodeSelector: `foo == "bar"`,
							VXLANMode:    api.VXLANModeAlways,
						},
					}, api.IPPool{
						ObjectMeta: metav1.ObjectMeta{Name: "ip-pool-2"},
						Spec: api.IPPoolSpec{
							Disabled:     false,
							CIDR:         "172.128.0.0/9",
							NodeSelector: `foo != "bar"`,
							VXLANMode:    api.VXLANModeAlways,
						},
					}}}

			// Execute and test assertions.
			cidrs := determineEnabledPoolCIDRs(n, pl, true)
			_, cidr1, _ := net.ParseCIDR("172.0.0.1/9")
			_, cidr2, _ := net.ParseCIDR("172.128.0.1/9")
			Expect(cidrs).To(ContainElement(*cidr1))
			Expect(cidrs).ToNot(ContainElement(*cidr2))
		})
	})
})
