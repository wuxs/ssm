package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateLegacyEnsuresSSMConfigAndInclude(t *testing.T) {
	home := setupHome(t, `Host existing
  HostName existing.internal
`)

	result, err := MigrateLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if result.MigratedCount != 0 || !result.IncludeAdded {
		t.Fatalf("result = %#v, want no configs and include added", result)
	}

	ssmData, err := os.ReadFile(filepath.Join(home, ".ssh", "ssm.config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ssmData), "# Managed by ssm migrate") {
		t.Fatalf("ssm.config missing managed header:\n%s", ssmData)
	}

	mainData, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainData), "Include ~/.ssh/ssm.config") {
		t.Fatalf("main config missing include:\n%s", mainData)
	}

	second, err := MigrateLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if second.IncludeAdded {
		t.Fatal("second migration added duplicate include")
	}
	mainData, err = os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(mainData), "Include ~/.ssh/ssm.config") != 1 {
		t.Fatalf("include duplicated:\n%s", mainData)
	}
}

func TestMigrateLegacyUpdatesExistingSSMConfigEntries(t *testing.T) {
	home := setupHome(t, `Include ~/.ssh/ssm.config
`)
	ssmPath := filepath.Join(home, ".ssh", "ssm.config")
	if err := os.WriteFile(ssmPath, []byte(`Host 192.168.1.159
  # SSM-Remark: kvm-k8s-01
  HostName 192.168.1.159
  User root
  Port 22
`), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if result.MigratedCount != 1 {
		t.Fatalf("MigratedCount = %d, want 1", result.MigratedCount)
	}

	data, err := os.ReadFile(ssmPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"Host kvm-k8s-01", "HostName 192.168.1.159", "User root", "Port 22"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ssm.config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Host 192.168.1.159\n") {
		t.Fatalf("ssm.config still contains old host alias:\n%s", got)
	}
}
