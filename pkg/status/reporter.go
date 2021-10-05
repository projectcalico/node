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

package status

import (
	"context"
	"reflect"
	"time"

	cerrors "github.com/projectcalico/libcalico-go/lib/errors"

	populator "github.com/projectcalico/node/pkg/status/populators"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcalico/libcalico-go/lib/options"

	log "github.com/sirupsen/logrus"

	apiv3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"

	client "github.com/projectcalico/libcalico-go/lib/clientv3"
)

// reporter contains all the data/method about reporting back node status based on a single node status resource.
// Each reporter has a goroutine which constantly reads node status and updates node status resource.
type reporter struct {
	// The name of the node status resource.
	name string

	// Internal client to operate on node status resource.
	client client.Interface

	// buffered channel to receive updates for the resource.
	ch chan *apiv3.CalicoNodeStatus

	// status holds latest version of node status resource.
	status *apiv3.CalicoNodeStatus

	// Interval and Time ticker that node status should be reported.
	interval uint32
	ticker   *time.Ticker

	// populators
	populators populatorRegistry

	// channel to indicate this reporter is not needed anymore.
	// It should start termination process.
	done chan struct{}

	// channel to indicate this reporter is terminated.
	term chan struct{}

	// New log entry.
	logCtx *log.Entry
}

// newReporter creates a reporter and start running a goroutine handling resource update.
// A new reporter is created when there is a new object.
func newReporter(name string,
	client client.Interface,
	populators populatorRegistry,
	request *apiv3.CalicoNodeStatus) *reporter {
	if request == nil {
		// Should not happen.
		log.Fatal("Trying to create a new reporter on a nil object")
	}
	r := &reporter{
		name:       name,
		client:     client,
		ch:         make(chan *apiv3.CalicoNodeStatus, 10),
		status:     request,
		populators: populators,
		done:       make(chan struct{}),
		term:       make(chan struct{}),
		logCtx:     log.WithField("object", name),
	}

	r.checkAndUpdateTicker(request.Spec.UpdatePeriodSeconds)

	go r.run()
	return r
}

// Check and set new ticker for the reporter.
// Make sure stop the old one first to GC old ticker.
func (r *reporter) checkAndUpdateTicker(pInterval *uint32) {
	var interval uint32
	if pInterval == nil {
		// Should not happen. Do nothing.
		return
	}
	interval = *pInterval

	if r.interval == interval {
		// no update needed.
		return
	}

	// Update ticker based on new interval value.
	// Stop ticker first.
	if r.ticker != nil {
		r.ticker.Stop()
	}
	r.interval = interval

	if interval == 0 {
		// Disable further updates.
		r.logCtx.Debug("Node status periodical update disabled")
	} else {
		r.logCtx.Infof("Node status update interval updated")
		r.ticker = time.NewTicker(time.Duration(interval) * time.Second)
	}
}

// Cleanup resources owned by this reporter.
func (r *reporter) cleanup() {
	r.ticker.Stop()
}

// KillAndWait sends done signal to reporter goroutine and wait until the
// goroutine of the reporter terminated.
func (r *reporter) KillAndWait() {
	r.done <- struct{}{}
	<-r.term
	r.logCtx.Debug("Node status reporter terminated.")
}

// Called when the caller needs to send a new version of request.
func (r *reporter) RequestUpdate(request *apiv3.CalicoNodeStatus) {
	r.ch <- request
}

// Return if the current status of the reporter has the same spec with
// the status passed in.
func (r *reporter) HasSameSpec(status *apiv3.CalicoNodeStatus) bool {
	return reflect.DeepEqual(r.status.Spec, status.Spec)
}

// run is the main reporting loop, it loops until done.
func (r *reporter) run() {
	r.logCtx.Debug("Start new goroutine to report node status")

	for {
		select {
		case latest := <-r.ch:
			// Received an update of node status resource.
			if latest.Name != r.name {
				r.logCtx.Errorf("node status reporter receive request with different name (%s), ignore it", latest.Name)
				break
			}

			r.status = latest
			r.checkAndUpdateTicker(latest.Spec.UpdatePeriodSeconds)
			// kick start node status update
			r.reportStatus()

		case <-r.ticker.C:
			// Todo check resource and update condition.
			r.reportStatus()

		case <-r.done:
			r.cleanup()
			r.term <- struct{}{}
			return
		}
	}
}

// reportStatus queries Bird or other components and update node status resource.
func (r *reporter) reportStatus() error {
	// The idea here is that we either update everything successfully or we update nothing.

	// Make a local copy first.
	status := *r.status

	for _, ipv := range []populator.IPFamily{populator.IPFamilyV4, populator.IPFamilyV6} {
		// Populate status from registered populators.
		for _, class := range r.status.Spec.Classes {
			p, ok := r.populators[ipv][class]
			if !ok {
				r.logCtx.Warningf("Wrong class (%s) requested for node status reporter", class)
				continue
			}
			err := p.Populate(&status)
			if err != nil {
				// If we hit any error, stop the entire update process.
				r.logCtx.WithError(err).Errorf("failed to populate status for ipv%s class %s", string(ipv), string(class))
				return err
			}
		}
	}

	r.logCtx.Debug("Status updated by populators")

	if reflect.DeepEqual(status.Status, r.status.Status) {
		// Nothing has changes since last time we updated.
		return nil
	}

	var err error
	var updatedResource *apiv3.CalicoNodeStatus
	// Update resource
	for i := 0; i < 3; i++ {
		ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
		status.Status.LastUpdated = metav1.Time{Time: time.Now()}
		updatedResource, err = r.client.CalicoNodeStatus().Update(ctx, &status, options.SetOptions{})
		if err != nil {
			if _, ok := err.(cerrors.ErrorResourceUpdateConflict); ok {
				r.logCtx.Warn("Node status resource update conflict - we are behind syncer update")

				// Just return and wait for syncer update to go through.
				return nil
			}
			log.WithError(err).Warnf("Failed to update node status resource; will retry")
			time.Sleep(1 * time.Second)
			continue
		}

		// Success!
		r.logCtx.Info("Latest status updated")
		r.status = updatedResource
		return nil
	}

	return err
}
