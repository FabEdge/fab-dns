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

package service_hub

import (
	"flag"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/fabedge/fab-dns/pkg/about"
)

func Execute() error {
	defer klog.Flush()

	opts := Options{}
	fs := pflag.CommandLine
	opts.AddFlags(fs)
	about.AddFlags(fs)
	addLogFlags(fs)

	pflag.Parse()

	about.DisplayAndExitIfRequested()

	if err := opts.Validate(); err != nil {
		log.Error(err, "invalid arguments found")
		return err
	}

	if err := opts.Complete(); err != nil {
		return err
	}

	return opts.Run()
}

func addLogFlags(fs *pflag.FlagSet) {
	local := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(local)

	fs.AddGoFlag(local.Lookup("v"))
}
