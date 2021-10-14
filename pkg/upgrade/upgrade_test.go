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

package upgrade

import (
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("verifyImagesShareRegistryPath",
	func(upgradeImage string, nodeImage string, noError bool) {
		err := verifyImagesShareRegistryPath(upgradeImage, nodeImage)
		Expect(err == nil).To(Equal(noError))
	},
	Entry("same prefix, tag", "docker.io/calico/windows-upgrade:v3.21.0", "docker.io/calico/node:v3.21.0", true),
	Entry("same prefix, digest1", "docker.io/calico/windows-upgrade:v3.21.0", "docker.io/calico/node@sha256:xxxxxxxx", true),
	Entry("same prefix, digest2", "docker.io/calico/windows-upgrade@sha256:aaaabbbb", "docker.io/calico/node@sha256:xxxxxxxx", true),
	Entry("diff prefix, tag", "quay.io/calico/windows-upgrade:v3.21.0", "docker.io/calico/node:v3.21.0", false),
	Entry("diff prefix, digest1", "docker.io/calico/windows-upgrade:v3.21.0", "quay.io/calico/node@sha256:xxxxxxxx", false),
	Entry("diff prefix, digest2", "docker.io/calico/windows-upgrade@sha256:aaaabbbb", "quay.io/calico/node@sha256:xxxxxxxx", false),
)
