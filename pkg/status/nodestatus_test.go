// Copyright (c) 2021 Tigera, Inc. All rights reserved.

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

package status_test

import (
	"context"
	"errors"
	"time"

	"github.com/projectcalico/libcalico-go/lib/backend/syncersv1/nodestatussyncer"

	apiv3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/libcalico-go/lib/options"
	"github.com/projectcalico/node/pkg/lifecycle/utils"
	populator "github.com/projectcalico/node/pkg/status/populators"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/projectcalico/node/pkg/status"

	log "github.com/sirupsen/logrus"

	"github.com/projectcalico/libcalico-go/lib/apiconfig"
	"github.com/projectcalico/libcalico-go/lib/backend"
	client "github.com/projectcalico/libcalico-go/lib/clientv3"
)

const (
	BootTimeFirst  = "2021-09-19 20:48:51"
	BootTimeSecond = "2021-09-19 20:48:52"
)

var _ = Describe("Node status FV tests", func() {
	// Create Calico client with k8s backend.
	cfg, err := apiconfig.LoadClientConfigFromEnvironment()
	Expect(err).NotTo(HaveOccurred())

	cfg.Spec = apiconfig.CalicoAPIConfigSpec{
		DatastoreType: apiconfig.Kubernetes,
		KubeConfig: apiconfig.KubeConfig{
			K8sAPIEndpoint:           "http://127.0.0.1:8080",
			K8sInsecureSkipTLSVerify: true,
		},
	}

	c, err := client.New(*cfg)
	Expect(err).NotTo(HaveOccurred())

	be, err := backend.NewClient(*cfg)
	Expect(err).NotTo(HaveOccurred())

	nodeName := utils.DetermineNodeName()
	name := "mynodestatus"

	v4Status := &apiv3.BGPDaemonStatus{
		State:                   apiv3.BGPDaemonStateReady,
		Version:                 "v0.3.3+birdv1.6.8",
		RouterID:                "172.17.0.0",
		LastBootTime:            BootTimeFirst,
		LastReconfigurationTime: "2021-09-19 20:48:56",
	}
	v6Status := &apiv3.BGPDaemonStatus{
		State:                   apiv3.BGPDaemonStateReady,
		Version:                 "v0.3.3+birdv1.6.8",
		RouterID:                "2001:20::8",
		LastBootTime:            BootTimeFirst,
		LastReconfigurationTime: "2021-09-19 20:48:56",
	}

	agentStatus := &apiv3.CalicoNodeAgentStatus{
		BIRDV4: *v4Status,
		BIRDV6: *v6Status,
	}

	var r *status.NodeStatusReporter
	var mock *mockBird

	BeforeEach(func() {
		err = be.Clean()
		Expect(err).ToNot(HaveOccurred())

		mock = newMockBird(v4Status, v6Status)
		r = status.NewNodeStatusReporter(nodeName, cfg, c, getPopulators(mock))
		mock.setLastBootTime(BootTimeFirst)
		mock.setError(nil)

		syncer := nodestatussyncer.New(be, r)
		syncer.Start()

		go r.Run()
	})

	AfterEach(func() {
		r.Stop()
	})

	getCurrentStatus := func() *apiv3.CalicoNodeStatus {
		status, err := c.CalicoNodeStatus().Get(context.Background(), name, options.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		return status
	}

	It("should update status just once if interval is 0", func() {
		// Create a node status request with interval of 0 seconds.
		createCalicoNodeStatus(c, nodeName, name, 0)

		// We should see an status update immediately.
		Eventually(func() *apiv3.CalicoNodeAgentStatus {
			status, err := c.CalicoNodeStatus().Get(context.Background(), name, options.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return &status.Status.Agent
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(agentStatus))

		saved := getCurrentStatus()

		// Update lastBootTime so new status can be populated if required
		mock.setLastBootTime(BootTimeSecond)

		// We should not see any update consistently for more than 10 seconds.
		Consistently(func() string {
			latest := getCurrentStatus()
			return latest.ResourceVersion
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(saved.ResourceVersion))
	})

	It("should get updated status object at the correct interval", func() {
		// Create a node status request with interval of 5 seconds.
		createCalicoNodeStatus(c, nodeName, name, 5)

		// We should see an status update immediately.
		Eventually(func() *apiv3.CalicoNodeAgentStatus {
			status, err := c.CalicoNodeStatus().Get(context.Background(), name, options.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return &status.Status.Agent
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(agentStatus))

		// Save value of lastUpdated.
		saved := getCurrentStatus()

		// Update lastBootTime so new status can be populated if required
		mock.setLastBootTime(BootTimeSecond)

		// Sleep 6 seconds so status should be updated.
		time.Sleep(6 * time.Second)

		// Should get a new update
		new := getCurrentStatus()

		// New update should have new values.
		Expect(new.Status.Agent.BIRDV4.LastBootTime).To(Equal(BootTimeSecond))
		Expect(new.Status.Agent.BIRDV6.LastBootTime).To(Equal(BootTimeSecond))
		Expect((&saved.Status.LastUpdated).Before(&new.Status.LastUpdated)).To(BeTrue())

		// We should not see any update consistently for more than 10 seconds.
		Consistently(func() string {
			latest := getCurrentStatus()
			return latest.ResourceVersion
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(new.ResourceVersion))
	})

	It("should not update status object if populator hitting an error", func() {
		// Create a node status request with interval of 5 seconds.
		createCalicoNodeStatus(c, nodeName, name, 5)

		// We should see an status update immediately.
		Eventually(func() *apiv3.CalicoNodeAgentStatus {
			status, err := c.CalicoNodeStatus().Get(context.Background(), name, options.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return &status.Status.Agent
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(agentStatus))

		// Save value of lastUpdated.
		new := getCurrentStatus()

		// Update lastBootTime so new status can be populated if required
		mock.setLastBootTime(BootTimeSecond)
		testErr := errors.New("mock a test error")
		mock.setError(&testErr)

		// We should not see any update consistently for more than 10 seconds.
		Consistently(func() string {
			latest := getCurrentStatus()
			return latest.ResourceVersion
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(new.ResourceVersion))
	})

	It("should create and release correct number of reporters", func() {
		// Create a node status request with interval of 10 seconds.
		createCalicoNodeStatus(c, nodeName, name, 5)
		createCalicoNodeStatus(c, nodeName, "new-status", 10)
		createCalicoNodeStatus(c, "wrong-node-name", "another-status", 10)

		// We should see two reporters.
		Eventually(func() int {
			return r.GetNumberOfReporters()
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(2))

		_, err := c.CalicoNodeStatus().Delete(context.Background(), name, options.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return r.GetNumberOfReporters()
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(1))

		_, err = c.CalicoNodeStatus().Delete(context.Background(), "new-status", options.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return r.GetNumberOfReporters()
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(0))

	})
})

func createCalicoNodeStatus(c client.Interface, node string, name string, interval int) {
	log.Info("Creating an CalicoNodeStatus")
	seconds := uint32(interval)
	_, err := c.CalicoNodeStatus().Create(
		context.Background(),
		&apiv3.CalicoNodeStatus{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: apiv3.CalicoNodeStatusSpec{
				Node: node,
				Classes: []apiv3.NodeStatusClassType{
					apiv3.NodeStatusClassTypeAgent,
				},
				UpdatePeriodSeconds: &seconds,
			},
		},
		options.SetOptions{},
	)
	Expect(err).NotTo(HaveOccurred())
}

// getPopulators returns PopulatorRegistry with mockBird.
func getPopulators(mock *mockBird) status.PopulatorRegistry {
	populators := make(map[populator.IPFamily]map[apiv3.NodeStatusClassType]populator.Interface)

	for _, ipv := range []populator.IPFamily{populator.IPFamilyV4, populator.IPFamilyV6} {
		populators[ipv] = make(map[apiv3.NodeStatusClassType]populator.Interface)
		populators[ipv][apiv3.NodeStatusClassTypeAgent] = mock
	}

	return populators
}

// mockBird implement populator interface to return bird status.
type mockBird struct {
	// Used to set the lastBootTime for status.
	lastBootTime string

	// Used to simulate an error condition.
	returnErr *error

	v4Status *apiv3.BGPDaemonStatus
	v6Status *apiv3.BGPDaemonStatus
}

func newMockBird(
	v4Status *apiv3.BGPDaemonStatus,
	v6Status *apiv3.BGPDaemonStatus) *mockBird {
	return &mockBird{
		v4Status: v4Status,
		v6Status: v6Status,
	}
}

func (b *mockBird) setLastBootTime(s string) {
	b.lastBootTime = s
}

func (b *mockBird) setError(e *error) {
	b.returnErr = e
}

func (b *mockBird) Populate(status *apiv3.CalicoNodeStatus) error {
	if b.returnErr != nil {
		return *b.returnErr
	}

	if b.v4Status != nil {
		b.v4Status.LastBootTime = b.lastBootTime
		status.Status.Agent.BIRDV4 = *b.v4Status
	}

	if b.v6Status != nil {
		b.v6Status.LastBootTime = b.lastBootTime
		status.Status.Agent.BIRDV6 = *b.v6Status
	}

	return nil
}

func (b *mockBird) Show() {
}
