package main

// build with external golang source code
// https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-external-golang-source-code

import (
	_ "github.com/FabEdge/fab-dns/pkg/fabdns"
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/coredns/coredns/coremain"
)

func init() {
	dnsserver.Directives = append([]string{"fabdns"}, dnsserver.Directives...)
}

func main() {
	coremain.Run()
}
