package main

// build with external golang source code
// https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-external-golang-source-code

import (
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/coredns/coredns/coremain"
	_ "github.com/fabedge/fab-dns/pkg/fabdns"
)

func init() {
	dnsserver.Directives = append([]string{"fabdns"}, dnsserver.Directives...)
}

func main() {
	coremain.Run()
}
