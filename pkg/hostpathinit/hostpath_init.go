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
package hostpathinit

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// Hostpath init should only need to be run when we are trying to
// run calico-node as non-root. This creates the directories that
// Calico needs on the host and changes their permissions so that
// the Calico user can access them.
func Run() {
	uidStr := os.Getenv("NODE_USER_ID")
	if uidStr == "" {
		// Default the UID to 1000
		uidStr = "1000"
	}

	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		log.Panicf("Failed to parse value for UID %s", uidStr)
	}

	// Create the calico directory in /var/lib/
	err = os.MkdirAll("/var/lib/calico/", 0700)
	if err != nil {
		log.Panic("Unable to create directory /var/lib/calico/")
	}

	// Change ownership of /var/lib/calico/ to our non-root user
	err = os.Chown("/var/lib/calico/", uid, 0)
	if err != nil {
		log.Panic("Unable to chown /var/lib/calico/")
	}

	// Create the calico directory in /var/run/
	err = os.MkdirAll("/var/run/calico/", 0700)
	if err != nil {
		log.Panic("Unable to create directory /var/run/calico/")
	}

	// Change ownership of /var/run/calico/ to our non-root user
	err = os.Chown("/var/run/calico/", uid, 0)
	if err != nil {
		log.Panic("Unable to chown /var/run/calico/")
	}

	// Create the calico directory in /var/log/
	err = os.MkdirAll("/var/log/calico/", 0700)
	if err != nil {
		log.Panic("Unable to create directory /var/log/calico/")
	}

	// Change ownership of /var/log/calico/ to  our non-root user
	err = os.Chown("/var/log/calico/", uid, 0)
	if err != nil {
		log.Panic("Unable to chown /var/log/calico/")
	}

	// Create the cni log directory in /var/log/calico/
	err = os.MkdirAll("/var/log/calico/cni", 0700)
	if err != nil {
		log.Panic("Unable to create directory /var/log/calico/cni")
	}

	// Change ownership of the cni log directory /var/log/calico/cni to  our non-root user
	err = os.Chown("/var/log/calico/cni", uid, 0)
	if err != nil {
		log.Panic("Unable to chown /var/log/calico/cni")
	}

	// Change ownership of all files in the cni log directory.
	// There will likely be files here since logs might have been created
	// separately by the CNI plugin.
	files, err := ioutil.ReadDir("/var/log/calico/cni/")
	if err != nil {
		log.Panic("Unable to read files in /var/log/calico/cni/ to change ownership")
	}

	for _, file := range files {
		err = os.Chown(fmt.Sprintf("/var/log/calico/cni/%s", file.Name()), uid, 0)
		if err != nil {
			log.Panicf("Unable to chown /var/log/calico/cni/%s", file.Name())
		}
	}
}
