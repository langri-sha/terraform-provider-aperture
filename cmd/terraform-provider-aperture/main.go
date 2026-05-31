// Command terraform-provider-aperture is the Terraform provider entrypoint
// for Aperture by Tailscale. The actual provider lives in
// github.com/langri-sha/aperture/internal/provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/langri-sha/aperture/internal/provider"
)

// These are populated at build time via -ldflags by goreleaser.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/langri-sha/aperture",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version, commit), opts); err != nil {
		log.Fatal(err.Error())
	}
}
