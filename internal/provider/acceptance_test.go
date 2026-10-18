package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/langri-sha/aperture/internal/fakegateway"
)

var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"aperture": providerserver.NewProtocol6WithError(New("test", "none")()),
}

const resourceAddress = "aperture_config.main"

func providerBlock(g *fakegateway.Gateway) string {
	return fmt.Sprintf("provider \"aperture\" {\n  endpoint = %q\n}\n", g.Endpoint())
}

const liveShapedConfig = `
resource "aperture_config" "main" {
  providers = {
    anthropic = {
      baseurl       = "https://api.anthropic.com"
      authorization = "x-api-key"
      auth_mode     = "passthrough"
      models        = ["claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5"]
      compatibility = {
        anthropic_messages = true
        openai_chat        = false
      }
    }
    codex = {
      baseurl   = "https://chatgpt.com/backend-api/codex"
      auth_mode = "passthrough"
      models    = ["gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark", "codex-auto-review"]
      compatibility = {
        openai_chat      = false
        openai_responses = true
      }
    }
  }

  grants = [
    {
      src          = ["*"]
      capabilities = [{ role = "admin" }]
    },
    {
      src = ["*"]
      capabilities = [
        { role = "user", models = "**" },
        { connectors = ["aperture/**"] },
      ]
    },
  ]

  hooks = {
    oso = {
      url = "https://api.osohq.com/api/agents/v1/model-request"
    }
  }
}
`

func storedDocumentContains(g *fakegateway.Gateway, parts ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		doc := strings.Join(strings.Fields(g.Document()), "")
		for _, p := range parts {
			if !strings.Contains(doc, strings.Join(strings.Fields(p), "")) {
				return fmt.Errorf("gateway document does not contain %s:\n%s", p, g.Document())
			}
		}
		return nil
	}
}

func fixtureGateway(t *testing.T) *fakegateway.Gateway {
	t.Helper()
	return fakegateway.New(t, gatewayFixture(t))
}

func TestAccConfig_ImportOfALiveShapedGatewayPlansClean(t *testing.T) {
	g := fixtureGateway(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             providerBlock(g) + liveShapedConfig,
				ResourceName:       resourceAddress,
				ImportState:        true,
				ImportStateId:      "default",
				ImportStatePersist: true,
			},
			{
				Config:   providerBlock(g) + liveShapedConfig,
				PlanOnly: true,
			},
		},
	})
}

func TestAccConfig_ApplyKeepsWhatTheConfigurationDoesNotMention(t *testing.T) {
	g := fakegateway.New(t, withExporters(gatewayFixture(t)))
	removingTheHook := strings.Replace(liveShapedConfig, `hooks = {
    oso = {
      url = "https://api.osohq.com/api/agents/v1/model-request"
    }
  }`, "", 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock(g) + removingTheHook,
				Check: resource.ComposeAggregateTestCheckFunc(
					storedDocumentContains(g,
						`"auth_mode": "passthrough"`,
						`"anthropic_messages": true`,
						`"connectors": ["aperture/**"]`,
						`"exporters": {"s3": {"bucket": "audit"}}`,
					),
					resource.TestCheckResourceAttr(resourceAddress, "providers.codex.compatibility.openai_responses", "true"),
				),
			},
			{
				Config:   providerBlock(g) + removingTheHook,
				PlanOnly: true,
			},
		},
	})
}

func withExporters(doc string) string {
	return strings.Replace(doc, `"providers": {`, `"exporters": {"s3": {"bucket": "audit"}}, "providers": {`, 1)
}

func TestAccConfig_UpdateWritesTheChange(t *testing.T) {
	g := fixtureGateway(t)
	withoutHook := strings.Replace(liveShapedConfig, `hooks = {
    oso = {
      url = "https://api.osohq.com/api/agents/v1/model-request"
    }
  }`, "", 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerBlock(g) + withoutHook},
			{
				Config: providerBlock(g) + strings.Replace(withoutHook, `"claude-haiku-5"]`, `"claude-haiku-5", "claude-next"]`, 1),
				Check:  storedDocumentContains(g, `"claude-next"`),
			},
		},
	})
}

func TestAccConfig_ApplyFailsRatherThanDropACredentialItCannotSee(t *testing.T) {
	g := fixtureGateway(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock(g) + liveShapedConfig,
				ExpectError: regexp.MustCompile(`aperture credential not managed`),
			},
		},
	})
	if len(g.Writes()) != 0 {
		t.Errorf("gateway was written: %v", g.Writes())
	}
}

func TestAccConfig_ApplyWritesTheConfiguredCredentialNeverTheMarker(t *testing.T) {
	g := fixtureGateway(t)
	withKey := strings.Replace(liveShapedConfig, `url = "https://api.osohq.com/api/agents/v1/model-request"`, `url    = "https://api.osohq.com/api/agents/v1/model-request"
      apikey = "configured-hook-key"`, 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock(g) + withKey,
				Check:  storedDocumentContains(g, `"apikey": "configured-hook-key"`),
			},
			{
				Config:   providerBlock(g) + withKey,
				PlanOnly: true,
			},
		},
	})
	for _, w := range g.Writes() {
		if strings.Contains(w, fakegateway.RedactedMarker) {
			t.Errorf("the redaction marker was written back: %s", w)
		}
	}
}

func TestAccConfig_PlanShowsADocumentChangedOutsideTerraform(t *testing.T) {
	g := fixtureGateway(t)
	withoutHook := strings.Replace(liveShapedConfig, `hooks = {
    oso = {
      url = "https://api.osohq.com/api/agents/v1/model-request"
    }
  }`, "", 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: providerBlock(g) + withoutHook},
			{
				PreConfig: func() {
					g.Replace(t, strings.Replace(g.Document(), `"gpt-5.5",`, `"gpt-5.5", "edited-in-console",`, 1))
				},
				Config:             providerBlock(g) + withoutHook,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccConfig_JSONSectionsAreManagedOnlyWhenSet(t *testing.T) {
	g := fakegateway.New(t, withExporters(gatewayFixture(t)))
	base := strings.Replace(liveShapedConfig, `hooks = {
    oso = {
      url = "https://api.osohq.com/api/agents/v1/model-request"
    }
  }`, "", 1)
	withDatabase := strings.Replace(base, "grants = [", `database = jsonencode({ captures = { retention = { duration = "24h" } } })

  grants = [`, 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock(g) + withDatabase,
				Check:  storedDocumentContains(g, `"database": {"captures": {"retention": {"duration": "24h"}}}`, `"exporters": {"s3": {"bucket": "audit"}}`),
			},
			{
				Config:   providerBlock(g) + withDatabase,
				PlanOnly: true,
			},
		},
	})
}

func TestAccConfig_ResourceExampleApplies(t *testing.T) {
	g := fakegateway.New(t, `{"grants": [{"src": ["*"], "app": {"tailscale.com/cap/aperture": [{"role": "admin"}]}}]}`)
	example, err := os.ReadFile("../../examples/resources/aperture_config/resource.tf")
	if err != nil {
		t.Fatal(err)
	}
	variables := `
variable "openai_api_key" {
  type    = string
  default = "sk-example"
}

variable "audit_hook_api_key" {
  type    = string
  default = "audit-example"
}
`
	config := providerBlock(g) + variables + string(example)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config, PlanOnly: true},
		},
	})
}
