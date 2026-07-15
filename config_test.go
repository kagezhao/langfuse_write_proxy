package main

import "testing"

func TestLoadYAMLParsesMultipleProjects(t *testing.T) {
	cfg, err := LoadYAML([]byte(`
server:
  listen_addr: ":9090"
  max_body_bytes: 1048576
  read_header_timeout: 5s
log:
  dir: "test-logs"
  max_days: 3
projects:
  - name: vm-a
    langfuse_base_url: https://langfuse-a.example.com/
    langfuse_public_key: pk-lf-a
    langfuse_secret_key: sk-lf-a
  - name: vm-b
    langfuse_base_url: https://langfuse-b.example.com/base
    langfuse_public_key: pk-lf-b
    langfuse_secret_key: sk-lf-b
`))
	if err != nil {
		t.Fatalf("LoadYAML() error = %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.MaxBodyBytes != 1048576 {
		t.Fatalf("MaxBodyBytes = %d", cfg.MaxBodyBytes)
	}
	if cfg.LogDir != "test-logs" {
		t.Fatalf("LogDir = %q", cfg.LogDir)
	}
	if cfg.LogMaxDays != 3 {
		t.Fatalf("LogMaxDays = %d", cfg.LogMaxDays)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("projects = %d", len(cfg.Projects))
	}
	if got := cfg.Projects[0].UpstreamURL.String(); got != "https://langfuse-a.example.com" {
		t.Fatalf("project 0 upstream = %q", got)
	}
	if got := cfg.Projects[1].UpstreamURL.String(); got != "https://langfuse-b.example.com/base" {
		t.Fatalf("project 1 upstream = %q", got)
	}
}

func TestLoadYAMLUsesDefaultLogConfig(t *testing.T) {
	cfg, err := LoadYAML([]byte(`
projects:
  - langfuse_base_url: https://langfuse-a.example.com
    langfuse_public_key: pk-lf-a
    langfuse_secret_key: sk-lf-a
`))
	if err != nil {
		t.Fatalf("LoadYAML() error = %v", err)
	}
	if cfg.LogDir != "logs" {
		t.Fatalf("LogDir = %q", cfg.LogDir)
	}
	if cfg.LogMaxDays != 7 {
		t.Fatalf("LogMaxDays = %d", cfg.LogMaxDays)
	}
}

func TestLoadYAMLRequiresProjects(t *testing.T) {
	_, err := LoadYAML([]byte(`server: { listen_addr: ":12000" }`))
	if err == nil {
		t.Fatal("expected missing projects error")
	}
}

func TestLoadYAMLRejectsDuplicatePublicKeys(t *testing.T) {
	_, err := LoadYAML([]byte(`
projects:
  - langfuse_base_url: https://langfuse-a.example.com
    langfuse_public_key: same-public-key
    langfuse_secret_key: sk-lf-a
  - langfuse_base_url: https://langfuse-b.example.com
    langfuse_public_key: same-public-key
    langfuse_secret_key: sk-lf-b
`))
	if err == nil {
		t.Fatal("expected duplicate public key error")
	}
}
