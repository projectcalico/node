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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/projectcalico/libcalico-go/lib/apiconfig"
	client "github.com/projectcalico/libcalico-go/lib/clientv3"

	"github.com/projectcalico/libcalico-go/lib/names"

	log "github.com/sirupsen/logrus"
)

const (
	defaultShutdownTimestampFileLinux   = `/var/lib/calico/shutdownTS`
	defaultShutdownTimestampFileWindows = `c:\CalicoWindows\shutdownTS`
	defaultNodenameFileLinux            = `/var/lib/calico/nodename`
	defaultNodenameFileWindows          = `c:\CalicoWindows\nodename`
)

// CreateCalicoClient loads the client config from environments and creates the
// Calico client.
func CreateCalicoClient() (*apiconfig.CalicoAPIConfig, client.Interface) {
	// Load the client config from environment.
	cfg, err := apiconfig.LoadClientConfig("")
	if err != nil {
		fmt.Printf("ERROR: Error loading datastore config: %s", err)
		os.Exit(1)
	}
	c, err := client.New(*cfg)
	if err != nil {
		fmt.Printf("ERROR: Error accessing the Calico datastore: %s", err)
		os.Exit(1)
	}

	return cfg, c
}

func shutdownTimestampFileName() string {
	fn := os.Getenv("CALICO_SHUTDOWN_TIMESTAMP_FILE")
	if fn == "" {
		if runtime.GOOS == "windows" {
			return defaultShutdownTimestampFileWindows
		} else {
			return defaultShutdownTimestampFileLinux
		}
	}
	return fn
}

func RemoveShutdownTimestampFile() error {
	filename := shutdownTimestampFileName()
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		log.WithError(err).Error("Failed to remove " + filename)
		return err
	}
	return nil
}

func SaveShutdownTimestamp() error {
	ts := time.Now().UTC().Format(time.RFC3339)
	filename := shutdownTimestampFileName()
	log.Infof("Writing shutdown timestamp %s to %s", ts, filename)
	if err := ioutil.WriteFile(filename, []byte(ts), 0644); err != nil {
		log.WithError(err).Error("Unable to write to " + filename)
		return err
	}
	return nil
}

// DetermineNodeName is called to determine the node name to use for this instance
// of calico/node.
func DetermineNodeName() string {
	var nodeName string
	var err error

	// Determine the name of this node.  Precedence is:
	// -  NODENAME
	// -  Value stored in our nodename file.
	// -  HOSTNAME (lowercase)
	// -  os.Hostname (lowercase).
	// We use the names.Hostname which lowercases and trims the name.
	if nodeName = strings.TrimSpace(os.Getenv("NODENAME")); nodeName != "" {
		log.Infof("Using NODENAME environment for node name %s", nodeName)
	} else if nodeName = nodenameFromFile(); nodeName != "" {
		log.Infof("Using stored node name %s from %s", nodeName, nodenameFileName())
	} else if nodeName = strings.ToLower(strings.TrimSpace(os.Getenv("HOSTNAME"))); nodeName != "" {
		log.Infof("Using HOSTNAME environment (lowercase) for node name %s", nodeName)
	} else if nodeName, err = names.Hostname(); err != nil {
		log.WithError(err).Error("Unable to determine hostname")
		terminate()
	} else {
		log.Warn("Using auto-detected node name. It is recommended that an explicit value is supplied using " +
			"the NODENAME environment variable.")
	}
	log.Infof("Determined node name: %s", nodeName)

	return nodeName
}

func nodenameFileName() string {
	fn := os.Getenv("CALICO_NODENAME_FILE")
	if fn == "" {
		if runtime.GOOS == "windows" {
			return defaultNodenameFileWindows
		} else {
			return defaultNodenameFileLinux
		}
	}
	return fn
}

// nodenameFromFile reads the nodename file if it exists and
// returns the nodename within.
func nodenameFromFile() string {
	filename := nodenameFileName()
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty string.
			log.Debug("File does not exist: " + filename)
			return ""
		}
		log.WithError(err).Error("Failed to read " + filename)
		terminate()
	}
	return string(data)
}

// Set Kubernetes NodeNetworkUnavailable to false when starting
// https://kubernetes.io/docs/concepts/architecture/nodes/#condition
func SetNodeNetworkUnavailableCondition(clientset kubernetes.Clientset,
	nodeName string,
	value bool,
	timeout time.Duration) error {
	log.Infof("Setting NetworkUnavailable to %t", value)

	var condition kapiv1.NodeCondition
	if value {
		condition = kapiv1.NodeCondition{
			Type:               kapiv1.NodeNetworkUnavailable,
			Status:             kapiv1.ConditionTrue,
			Reason:             "CalicoIsDown",
			Message:            "Calico is shutting down on this node",
			LastTransitionTime: metav1.Now(),
			LastHeartbeatTime:  metav1.Now(),
		}
	} else {
		condition = kapiv1.NodeCondition{
			Type:               kapiv1.NodeNetworkUnavailable,
			Status:             kapiv1.ConditionFalse,
			Reason:             "CalicoIsUp",
			Message:            "Calico is running on this node",
			LastTransitionTime: metav1.Now(),
			LastHeartbeatTime:  metav1.Now(),
		}
	}

	raw, err := json.Marshal(&[]kapiv1.NodeCondition{condition})
	if err != nil {
		return err
	}
	patch := []byte(fmt.Sprintf(`{"status":{"conditions":%s}}`, raw))
	to := time.After(timeout)
	for {
		select {
		case <-to:
			err = fmt.Errorf("timed out patching node, last error was: %s", err.Error())
			return err
		default:
			_, err = clientset.CoreV1().Nodes().PatchStatus(context.Background(), nodeName, patch)
			if err != nil {
				log.WithError(err).Warnf("Failed to set NetworkUnavailable; will retry")
			} else {
				// Success!
				return nil
			}
		}
	}
}
