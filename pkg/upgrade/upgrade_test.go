// Copyright (c) 2018,2021 Tigera, Inc. All rights reserved.

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

var _ = DescribeTable("calicoUpgradeImageRegex",
	func(containerImage string, expectMatch bool) {
		Expect(calicoUpgradeImageRegex.Match([]byte(containerImage))).To(Equal(expectMatch))
	},
	Entry("standard image", "docker.io/calico/windows-upgrade:latest", true),
	Entry("standard image", "quay.io/calico/windows-upgrade:v3.21.0", true),
	Entry("standard image", "quay.io/calico/windows-upgrade:v3.21.0-test", true),
	Entry("custom registry", "example-registry.com/calico/windows-upgrade:v3.21.0", true),
	Entry("registry with port", "example.com:5555/calico/windows-upgrade:v3.21.0", true),
	Entry("no tag", "docker.io/calico/windows-upgrade", false),
	Entry("no registry", "calico/windows-upgrade:v3.21.0", false),
)
