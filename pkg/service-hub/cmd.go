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
