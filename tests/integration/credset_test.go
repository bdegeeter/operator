//go:build integration

package integration_test

import (
	. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"
)

var _ = PDescribe("CredSet create", func() {
	Context("when a new CredentialSet resource is created", func() {
		It("should run porter", func() {
			By("creating an agent action", func() {
				Log("create a credential set")
			})
		})
	})
})
var _ = PDescribe("CredSet update", func() {
	Context("when a new CredentialSet resource is updated", func() {
		It("should run porter apply", func() {
			By("creating an agent action", func() {
				Log("update a credential set")
			})
		})
	})
})

var _ = PDescribe("CredSet delete", func() {})

var _ = PDescribe("New Installation with CredentialSet", func() {})
