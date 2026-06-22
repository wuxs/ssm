package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/wuxs/ssm/pkg/config"
)

func TestNewConnectionRowFormatsConfigFields(t *testing.T) {
	lastUsed := time.Date(2026, 6, 15, 16, 21, 0, 0, time.UTC).Format(time.RFC3339)
	cfg := config.SSHConfig{
		Host:       "api.prod",
		Username:   "deploy",
		Port:       "22",
		PrivateKey: "/home/deploy/.ssh/id_rsa",
		Password:   "secret",
		Remark:     "production api",
		LastUsed:   lastUsed,
	}

	row := newConnectionRow(cfg)

	if row.Key != "deploy@api.prod:22" {
		t.Fatalf("Key = %q, want %q", row.Key, "deploy@api.prod:22")
	}
	if row.Status != StatusUnknown {
		t.Fatalf("Status = %q, want %q", row.Status, StatusUnknown)
	}
	if row.Remark != "production api" {
		t.Fatalf("Remark = %q, want %q", row.Remark, "production api")
	}
	if row.LastUsed != "2026-06-15 16:21" {
		t.Fatalf("LastUsed = %q, want %q", row.LastUsed, "2026-06-15 16:21")
	}
	if row.HasPassword != "yes" {
		t.Fatalf("HasPassword = %q, want yes", row.HasPassword)
	}
	if row.HasKey != "yes" {
		t.Fatalf("HasKey = %q, want yes", row.HasKey)
	}
	if row.Auth != "password + key" {
		t.Fatalf("Auth = %q, want %q", row.Auth, "password + key")
	}
}

func TestNewConnectionRowUsesAliasAsRemarkAndHostNameAsHost(t *testing.T) {
	cfg := config.SSHConfig{Alias: "kvm-k8s-01", Host: "192.168.1.159", Username: "root", Port: "22"}

	row := newConnectionRow(cfg)

	if row.Remark != "kvm-k8s-01" {
		t.Fatalf("Remark = %q, want alias", row.Remark)
	}
	if row.Key != "root@192.168.1.159:22" {
		t.Fatalf("Key = %q, want actual host key", row.Key)
	}
}

func TestNewConnectionRowFormatsMissingFields(t *testing.T) {
	cfg := config.SSHConfig{Host: "10.0.1.8", Username: "root", Port: "2222"}

	row := newConnectionRow(cfg)

	if row.LastUsed != "never" {
		t.Fatalf("LastUsed = %q, want never", row.LastUsed)
	}
	if row.HasPassword != "no" {
		t.Fatalf("HasPassword = %q, want no", row.HasPassword)
	}
	if row.HasKey != "no" {
		t.Fatalf("HasKey = %q, want no", row.HasKey)
	}
	if row.Auth != "none" {
		t.Fatalf("Auth = %q, want none", row.Auth)
	}
}

func TestRenderRowsShowsCompactTableAndSelection(t *testing.T) {
	rows := []ConnectionRow{
		{Remark: "prod api", Key: "deploy@api.prod:22", Status: StatusOnline, LastUsed: "2026-06-15 16:21", HasPassword: "yes", HasKey: "yes", Auth: "password + key"},
		{Remark: "db", Key: "root@10.0.1.8:2222", Status: StatusOffline, LastUsed: "never", HasPassword: "no", HasKey: "yes", Auth: "key"},
	}

	view := renderRows(rows, 1)

	for _, want := range []string{"Remark", "Host", "Status", "Last Used", "Pwd", "Key", "Auth", "> db", "root@10.0.1.8:2222", "offline"} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered table missing %q:\n%s", want, view)
		}
	}
}

func TestRenderRowsAlignsHeadersWithContentColumns(t *testing.T) {
	rows := []ConnectionRow{
		{Remark: "prod api", Key: "deploy@api.prod:22", Status: StatusOnline, LastUsed: "2026-06-15 16:21", HasPassword: "pwd", HasKey: "key", Auth: "password + key"},
	}

	view := renderRows(rows, 0)
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("rendered table has %d lines, want at least 2:\n%s", len(lines), view)
	}
	header := lines[0]
	row := lines[1]

	checks := map[string]string{
		"Remark":    "prod api",
		"Host":      "deploy@api.prod:22",
		"Status":    string(StatusOnline),
		"Last Used": "2026-06-15 16:21",
		"Pwd":       "pwd",
		"Key":       "key",
		"Auth":      "password + key",
	}
	for headerValue, rowValue := range checks {
		headerIndex := strings.Index(header, headerValue)
		rowIndex := strings.Index(row, rowValue)
		if headerIndex == -1 || rowIndex == -1 {
			t.Fatalf("missing %q or %q in rendered table:\n%s", headerValue, rowValue, view)
		}
		if headerIndex != rowIndex {
			t.Fatalf("column %q starts at %d, row value %q starts at %d:\n%s", headerValue, headerIndex, rowValue, rowIndex, view)
		}
	}
}

func TestRenderRowsStyledAddsHighlightAndStatusColors(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	rows := []ConnectionRow{
		{Remark: "prod api", Key: "deploy@api.prod:22", Status: StatusOnline, LastUsed: "2026-06-15 16:21", HasPassword: "yes", HasKey: "yes", Auth: "password + key"},
		{Remark: "db", Key: "root@10.0.1.8:2222", Status: StatusOffline, LastUsed: "never", HasPassword: "no", HasKey: "yes", Auth: "key"},
		{Key: "admin@jump-box:22", Status: StatusChecking, LastUsed: "never", HasPassword: "yes", HasKey: "no", Auth: "password"},
		{Key: "ops@unknown:22", Status: StatusUnknown, LastUsed: "never", HasPassword: "no", HasKey: "no", Auth: "none"},
	}

	view := renderRowsStyled(rows, 1)

	for _, want := range []string{"\x1b[", "online", "offline", "checking", "unknown", "> db", "root@10.0.1.8:2222"} {
		if !strings.Contains(view, want) {
			t.Fatalf("styled table missing %q:\n%q", want, view)
		}
	}
}

func TestRenderRowsStyledKeepsVisibleColumnsAligned(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	rows := []ConnectionRow{
		{Remark: "prod api", Key: "deploy@api.prod:22", Status: StatusOnline, LastUsed: "2026-06-15 16:21", HasPassword: "pwd", HasKey: "key", Auth: "password + key"},
	}

	view := stripANSI(renderRowsStyled(rows, 0))
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("rendered table has %d lines, want at least 2:\n%s", len(lines), view)
	}
	header := lines[0]
	row := lines[1]

	checks := map[string]string{
		"Remark":    "prod api",
		"Host":      "deploy@api.prod:22",
		"Status":    string(StatusOnline),
		"Last Used": "2026-06-15 16:21",
		"Pwd":       "pwd",
		"Key":       "key",
		"Auth":      "password + key",
	}
	for headerValue, rowValue := range checks {
		headerIndex := strings.Index(header, headerValue)
		rowIndex := strings.Index(row, rowValue)
		if headerIndex == -1 || rowIndex == -1 {
			t.Fatalf("missing %q or %q in rendered table:\n%s", headerValue, rowValue, view)
		}
		if headerIndex != rowIndex {
			t.Fatalf("column %q starts at %d, row value %q starts at %d:\n%s", headerValue, headerIndex, rowValue, rowIndex, view)
		}
	}
}

func TestRenderRowsKeepsPlainTextWithoutANSI(t *testing.T) {
	rows := []ConnectionRow{{Key: "deploy@api.prod:22", Status: StatusOnline, LastUsed: "never", HasPassword: "yes", HasKey: "yes", Auth: "password + key"}}

	view := renderRows(rows, 0)

	if strings.Contains(view, "\x1b[") {
		t.Fatalf("plain table contains ANSI escapes:\n%q", view)
	}
}

func stripANSI(value string) string {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiPattern.ReplaceAllString(value, "")
}
