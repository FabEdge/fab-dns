package dns_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFabdns(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fabdns Suite")
}
