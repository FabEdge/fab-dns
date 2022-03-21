package main

import (
	"os"

	service_hub "github.com/fabedge/fab-dns/pkg/service-hub"
)

func main() {
	if err := service_hub.Execute(); err != nil {
		os.Exit(1)
	}
}
