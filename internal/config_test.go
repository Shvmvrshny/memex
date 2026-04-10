package memex

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Unsetenv("IDENTITY_PATH")
	os.Unsetenv("KG_PATH")

	cfg := LoadConfig()

	home, _ := os.UserHomeDir()
	wantIdentity := home + "/.memex/identity.md"
	wantKG := home + "/.memex/knowledge_graph.db"

	if cfg.IdentityPath != wantIdentity {
		t.Errorf("IdentityPath = %q, want %q", cfg.IdentityPath, wantIdentity)
	}
	if cfg.KGPath != wantKG {
		t.Errorf("KGPath = %q, want %q", cfg.KGPath, wantKG)
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	os.Setenv("IDENTITY_PATH", "/custom/identity.md")
	os.Setenv("KG_PATH", "/custom/kg.db")
	defer os.Unsetenv("IDENTITY_PATH")
	defer os.Unsetenv("KG_PATH")

	cfg := LoadConfig()

	if cfg.IdentityPath != "/custom/identity.md" {
		t.Errorf("IdentityPath = %q, want /custom/identity.md", cfg.IdentityPath)
	}
	if cfg.KGPath != "/custom/kg.db" {
		t.Errorf("KGPath = %q, want /custom/kg.db", cfg.KGPath)
	}
}
