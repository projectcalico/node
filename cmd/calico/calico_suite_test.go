// Copyright (c) 2018 Tigera, Inc. All rights reserved.

package main_test

import (
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"testing"
)

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("../../report/calico_cni_config_suite.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Calico CNI Config Suite", []Reporter{junitReporter})
}
