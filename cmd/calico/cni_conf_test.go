// Copyright (c) 2018-2020 Tigera, Inc. All rights reserved.

package main_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CNI config template tests", func() {
	It("should be valid JSON", func() {
		f, err := ioutil.ReadFile("../../windows-packaging/TigeraCalico/cni.conf.template")
		Expect(err).NotTo(HaveOccurred())

		// __VNI__ is a placeholder for a bare int so we need to swap it for something valid.
		f = bytes.Replace(f, []byte("__VNI__"), []byte("0"), -1)

		var data map[string]interface{}
		err = json.Unmarshal(f, &data)
		Expect(err).NotTo(HaveOccurred())
	})
})
