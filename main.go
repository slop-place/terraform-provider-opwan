package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/slop-place/terraform-provider-opwan/internal/provider"
)

//go:generate go tool tfplugindocs generate --provider-name opwan

const PROVIDER_ADDRESS = "registry.terraform.io/slop-place/opwan"

// Set by goreleaser.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: PROVIDER_ADDRESS,
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
