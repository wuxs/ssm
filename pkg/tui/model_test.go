package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wuxs/ssm/pkg/config"
)

func TestModelMovesSelectionWithArrowKeys(t *testing.T) {
	m := NewModel([]config.SSHConfig{
		{Host: "one", Username: "root", Port: "22"},
		{Host: "two", Username: "root", Port: "22"},
	}, nil)

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.selected != 0 {
		t.Fatalf("selected = %d, want 0", m.selected)
	}
}

func TestNewModelMarksRowsCheckingForInitialRefresh(t *testing.T) {
	m := NewModel([]config.SSHConfig{{Host: "one", Username: "root", Port: "22"}}, nil)

	if m.rows[0].Status != StatusChecking {
		t.Fatalf("Status = %q, want %q", m.rows[0].Status, StatusChecking)
	}
}

func TestModelEnterSelectsCurrentConfigAndQuits(t *testing.T) {
	configs := []config.SSHConfig{
		{Host: "one", Username: "root", Port: "22"},
		{Host: "two", Username: "deploy", Port: "2222"},
	}
	m := NewModel(configs, nil)
	m.selected = 1

	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})

	if !m.Done {
		t.Fatal("Done = false, want true")
	}
	if m.SelectedConfig == nil || m.SelectedConfig.Host != "two" {
		t.Fatalf("SelectedConfig = %#v, want host two", m.SelectedConfig)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want quit command")
	}
}

func TestModelRMarksRowsCheckingAndSchedulesRefresh(t *testing.T) {
	m := NewModel([]config.SSHConfig{{Host: "one", Username: "root", Port: "22"}}, nil)
	m.rows[0].Status = StatusOffline

	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if m.rows[0].Status != StatusChecking {
		t.Fatalf("Status = %q, want %q", m.rows[0].Status, StatusChecking)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want refresh command")
	}
}

func TestModelStatusBatchUpdatesRows(t *testing.T) {
	m := NewModel([]config.SSHConfig{{Host: "one", Username: "root", Port: "22"}}, nil)

	m, _ = updateModel(m, statusBatchMsg{Results: map[string]ConnectionStatus{"root@one:22": StatusOnline}})

	if m.rows[0].Status != StatusOnline {
		t.Fatalf("Status = %q, want %q", m.rows[0].Status, StatusOnline)
	}
}

func TestModelDeleteRemovesSelectedConfig(t *testing.T) {
	var deleted string
	m := NewModel([]config.SSHConfig{
		{Host: "one", Username: "root", Port: "22"},
		{Host: "two", Username: "root", Port: "22"},
	}, func(key string) error {
		deleted = key
		return nil
	})
	m.selected = 1

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if deleted != "root@two:22" {
		t.Fatalf("deleted = %q, want %q", deleted, "root@two:22")
	}
	if len(m.rows) != 1 || m.rows[0].Key != "root@one:22" {
		t.Fatalf("rows = %#v, want only root@one:22", m.rows)
	}
}

func TestModelEditsSelectedRemark(t *testing.T) {
	var saved *config.SSHConfig
	m := NewModel([]config.SSHConfig{{Host: "one", Username: "root", Port: "22", Remark: "old"}}, nil)
	m.saveConfig = func(cfg *config.SSHConfig) error {
		copy := *cfg
		saved = &copy
		return nil
	}

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prod")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("api")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})

	if saved == nil || saved.Remark != "prod apiq" {
		t.Fatalf("saved = %#v, want remark prod apiq", saved)
	}
	if m.rows[0].Remark != "prod apiq" || m.rows[0].Config.Remark != "prod apiq" {
		t.Fatalf("row = %#v, want updated remark", m.rows[0])
	}
	if m.editingRemark {
		t.Fatal("editingRemark = true, want false")
	}
	if m.Done {
		t.Fatal("Done = true, want false while editing consumes q as text")
	}
}

func TestModelEmptyViewShowsGuidance(t *testing.T) {
	m := NewModel(nil, nil)

	view := m.View()

	for _, want := range []string{"No saved connections", "ssm user@host", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
