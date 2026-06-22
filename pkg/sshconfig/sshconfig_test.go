package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupHome(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestResolveUsesSSHConfigAndSSMMetadata(t *testing.T) {
	home := setupHome(t, `Host jump
  HostName jump.internal
  User jumpuser

Host prod
  # SSM-Remark: production api
  # SSM-Password: base64:c2VjcmV0
  HostName 10.0.0.12
  User deploy
  Port 2222
  IdentityFile ~/.ssh/prod_key
  ProxyJump jump

Host *
  User fallback
  Port 2200
`)

	cfg, err := Resolve("prod", Options{})
	if err != nil {
		t.Fatal(err)
	}

	checks := map[string]string{
		"Alias":      cfg.Alias,
		"Host":       cfg.Host,
		"Username":   cfg.Username,
		"Port":       cfg.Port,
		"PrivateKey": cfg.PrivateKey,
		"ProxyJump":  cfg.ProxyJump,
		"Remark":     cfg.Remark,
		"Password":   cfg.Password,
	}
	want := map[string]string{
		"Alias":      "prod",
		"Host":       "10.0.0.12",
		"Username":   "deploy",
		"Port":       "2222",
		"PrivateKey": filepath.Join(home, ".ssh", "prod_key"),
		"ProxyJump":  "jump",
		"Remark":     "production api",
		"Password":   "secret",
	}
	for key, got := range checks {
		if got != want[key] {
			t.Fatalf("%s = %q, want %q", key, got, want[key])
		}
	}
}

func TestResolveCommandLineOptionsOverrideSSHConfig(t *testing.T) {
	setupHome(t, `Host prod
  HostName 10.0.0.12
  User deploy
  Port 2222
  IdentityFile ~/.ssh/prod_key
  ProxyJump jump
`)

	cfg, err := Resolve("root@prod", Options{Port: "22", IdentityFile: "/tmp/key", ProxyJump: "other-jump"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "root" || cfg.Port != "22" || cfg.PrivateKey != "/tmp/key" || cfg.ProxyJump != "other-jump" {
		t.Fatalf("cfg = %#v, want command line overrides", cfg)
	}
}

func TestResolveUsesHostAliasAsRemarkFallback(t *testing.T) {
	setupHome(t, `Host kvm-k8s-01
  HostName 192.168.1.159
`)

	cfg, err := Resolve("kvm-k8s-01", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alias != "kvm-k8s-01" || cfg.Remark != "kvm-k8s-01" || cfg.Host != "192.168.1.159" {
		t.Fatalf("cfg = %#v, want alias as remark and HostName as host", cfg)
	}
}

func TestListSkipsWildcardHostsAndIncludesConcreteAliases(t *testing.T) {
	setupHome(t, `Host prod staging
  HostName app.internal
  User deploy

Host *.internal
  User ops

Host *
  User fallback
`)

	configs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	var aliases []string
	for _, cfg := range configs {
		aliases = append(aliases, cfg.Alias)
	}
	got := strings.Join(aliases, ",")
	if got != "prod,staging" {
		t.Fatalf("aliases = %q, want prod,staging", got)
	}
}

func TestSaveRemarkUpdatesSSHConfigComment(t *testing.T) {
	home := setupHome(t, `Host prod
  HostName 10.0.0.12
  User deploy
`)

	cfg, err := Resolve("prod", Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Remark = "new remark"
	if err := SaveRemark(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# SSM-Remark: new remark") {
		t.Fatalf("config missing remark:\n%s", data)
	}
}

func TestDeleteRemovesSingleAliasFromMultiHostBlock(t *testing.T) {
	home := setupHome(t, `Host prod staging
  HostName app.internal
  User deploy
`)

	if err := Delete("prod"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "Host prod") || !strings.Contains(got, "Host staging") {
		t.Fatalf("unexpected config after delete:\n%s", got)
	}
}

func TestResolveLoadsIncludedSSHConfigFiles(t *testing.T) {
	home := setupHome(t, `Include conf.d/*.conf

Host *
  User fallback
`)
	includeDir := filepath.Join(home, ".ssh", "conf.d")
	if err := os.MkdirAll(includeDir, 0700); err != nil {
		t.Fatal(err)
	}
	includePath := filepath.Join(includeDir, "xpark.conf")
	if err := os.WriteFile(includePath, []byte(`Host xpark-e274
  HostName 10.10.20.30
  User wuxs
  Port 2222
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve("xpark-e274", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "10.10.20.30" || cfg.Username != "wuxs" || cfg.Port != "2222" {
		t.Fatalf("cfg = %#v, want included host values", cfg)
	}

	cfg.Remark = "included remark"
	if err := SaveRemark(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(includePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# SSM-Remark: included remark") {
		t.Fatalf("included file missing remark:\n%s", data)
	}
}
