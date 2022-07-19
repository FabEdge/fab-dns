// Copyright 2021 FabEdge Team
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package framework

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/onsi/ginkgo/config"
)

type PreserveResourcesMode string

const (
	PreserveResourcesAlways    PreserveResourcesMode = "always"
	PreserveResourcesNever     PreserveResourcesMode = "never"
	PreserveResourcesOnFailure PreserveResourcesMode = "fail"
)

type Context struct {
	KubeConfigsDir    string
	FabdnsZone        string
	EdgeLabels        string
	GenReport         bool
	ReportFile        string
	RequestTimeout    time.Duration
	WaitTimeout       int64
	CurlTimeout       int64
	NetToolImage      string
	IPv6Enabled       bool
	PreserveResources string
	ShowExecError     bool
}

var TestContext Context

func RegisterAndHandleFlags() {
	flag.StringVar(&TestContext.KubeConfigsDir, "kube-configs-dir", "",
		"The path to a directory which contains all kubeconfig files of clusters to test")
	flag.StringVar(&TestContext.FabdnsZone, "fabdns-zone", "global",
		"The root zone in fabdns for DNS resolving")
	flag.StringVar(&TestContext.PreserveResources, "preserve-resources", string(PreserveResourcesOnFailure),
		"Whether preserve test resources, options: always, never, fail")
	flag.BoolVar(&TestContext.GenReport, "gen-report", false,
		"Whether generate report file, default: false")
	flag.StringVar(&TestContext.ReportFile, "report-file", "fabdns-e2e-test-report.txt",
		"The file to write test result")
	flag.Int64Var(&TestContext.WaitTimeout, "wait-timeout", 30,
		"How long to wait for test resources are ready. Unit: seconds")
	flag.Int64Var(&TestContext.CurlTimeout, "curl-timeout", 30,
		"Maxtime for curl to finish. Unit: seconds")
	flag.StringVar(&TestContext.NetToolImage, "net-tool-image", "fabedge/net-tool:v0.1.0",
		"The net-tool image")
	flag.BoolVar(&TestContext.ShowExecError, "show-exec-error", false,
		"display error of executing curl or ping")
	flag.StringVar(&TestContext.EdgeLabels, "edge-labels", "node-role.kubernetes.io/edge",
		"Labels to filter edge nodes, (e.g. key1,key2=,key3=value3)")
	flag.BoolVar(&TestContext.IPv6Enabled, "6", false, "Whether to test IPv6 services")

	flag.Parse()

	// Turn on verbose by default to get spec names
	config.DefaultReporterConfig.Verbose = true

	pr := PreserveResourcesMode(TestContext.PreserveResources)
	if pr != PreserveResourcesNever &&
		pr != PreserveResourcesAlways &&
		pr != PreserveResourcesOnFailure {
		fatalf("unknown preserve resources mode: %s", pr)
	}

	if len(TestContext.KubeConfigsDir) == 0 {
		fatalf("multi cluster kubeconfig dir is empty")
	}

	if len(TestContext.FabdnsZone) == 0 {
		fatalf("fabdns root zone is empty")
	} else {
		if _, ok := dns.IsDomainName(TestContext.FabdnsZone); !ok {
			fatalf("fabdns root zone is invalid")
		}
	}

	if TestContext.WaitTimeout <= 0 {
		fatalf("wait-timeout is too small")
	}

	if TestContext.CurlTimeout <= 0 {
		fatalf("curl-timeout is too small")
	}

	_, err := parseLabels(TestContext.EdgeLabels)
	if err != nil {
		fatalf("invalid edge labels: %s", err)
	}
}

func parseLabels(labels string) (map[string]string, error) {
	labels = strings.TrimSpace(labels)

	parsedEdgeLabels := make(map[string]string)
	for _, label := range strings.Split(labels, ",") {
		parts := strings.Split(label, "=")
		switch len(parts) {
		case 1:
			parsedEdgeLabels[parts[0]] = ""
		case 2:
			if parts[0] == "" {
				return nil, fmt.Errorf("label's key must not be empty")
			}
			parsedEdgeLabels[parts[0]] = parts[1]
		default:
			return nil, fmt.Errorf("wrong edge label format: %s", strings.Join(parts, "="))
		}
	}

	return parsedEdgeLabels, nil
}

func fatalf(format string, args ...interface{}) {
	Failf(format, args...)
	os.Exit(1)
}
