package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wuxs/ssm/pkg/config"
	"github.com/wuxs/ssm/pkg/sshconfig"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	messageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type DeleteFunc func(key string) error
type SaveConfigFunc func(cfg *config.SSHConfig) error

type Model struct {
	rows           []ConnectionRow
	selected       int
	deleteConfig   DeleteFunc
	saveConfig     SaveConfigFunc
	editingRemark  bool
	editValue      string
	message        string
	Done           bool
	SelectedConfig *config.SSHConfig
}

func NewModel(configs []config.SSHConfig, deleteConfig DeleteFunc) Model {
	if deleteConfig == nil {
		deleteConfig = sshconfig.Delete
	}
	saveConfig := SaveConfigFunc(sshconfig.SaveRemark)
	rows := rowsFromConfigs(configs)
	for i := range rows {
		rows[i].Status = StatusChecking
	}
	return Model{
		rows:         rows,
		deleteConfig: deleteConfig,
		saveConfig:   saveConfig,
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	return m.startStatusRefresh()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := updateModel(m, msg)
	return next, cmd
}

func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editingRemark {
			return m.updateRemarkEdit(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.Done = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.rows)-1 {
				m.selected++
			}
			return m, nil
		case "enter":
			if len(m.rows) == 0 {
				return m, nil
			}
			cfg := m.rows[m.selected].Config
			m.SelectedConfig = &cfg
			m.Done = true
			return m, tea.Quit
		case "r":
			if len(m.rows) == 0 {
				return m, nil
			}
			return m.markChecking(), m.startStatusRefresh()
		case "e":
			return m.startRemarkEdit(), nil
		case "d":
			return m.deleteSelected()
		}
	case statusBatchMsg:
		for i := range m.rows {
			if status, ok := msg.Results[m.rows[i].Key]; ok {
				m.rows[i].Status = status
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.rows) == 0 {
		return titleStyle.Render("SSM Connections") + "\n\nNo saved connections.\nConnect once with: " + keyStyle.Render("ssm user@host") + "\n\n" + keyStyle.Render("q") + " quit"
	}

	online, offline := m.statusCounts()
	var b strings.Builder
	b.WriteString(titleStyle.Render("SSM Connections"))
	b.WriteString("                         ")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("%d total | %d online | %d offline", len(m.rows), online, offline)))
	b.WriteString("\n\n")
	b.WriteString(renderRowsStyled(m.rows, m.selected))
	b.WriteString("\n\n")
	b.WriteString(keyStyle.Render("Enter"))
	b.WriteString(" connect | ")
	b.WriteString(keyStyle.Render("r"))
	b.WriteString(" refresh | ")
	b.WriteString(keyStyle.Render("d"))
	b.WriteString(" delete | ")
	b.WriteString(keyStyle.Render("e"))
	b.WriteString(" edit remark | ")
	b.WriteString(keyStyle.Render("q"))
	b.WriteString(" quit | up/down select")
	if m.editingRemark {
		b.WriteString("\n")
		b.WriteString(messageStyle.Render(fmt.Sprintf("Remark: %s", m.editValue)))
		b.WriteString("\n")
		b.WriteString(keyStyle.Render("Enter"))
		b.WriteString(" save | ")
		b.WriteString(keyStyle.Render("Esc"))
		b.WriteString(" cancel")
	}
	if m.message != "" {
		b.WriteString("\n")
		b.WriteString(messageStyle.Render(m.message))
	}
	return b.String()
}

func (m Model) startRemarkEdit() Model {
	if len(m.rows) == 0 {
		return m
	}
	m.editingRemark = true
	m.editValue = m.rows[m.selected].Remark
	m.message = ""
	return m
}

func (m Model) updateRemarkEdit(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.Done = true
		return m, tea.Quit
	case tea.KeyEnter:
		return m.saveRemarkEdit()
	case tea.KeyEsc:
		m.editingRemark = false
		m.editValue = ""
		m.message = "Remark edit canceled"
		return m, nil
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.editValue) > 0 {
			runes := []rune(m.editValue)
			m.editValue = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.editValue += string(msg.Runes)
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) saveRemarkEdit() (Model, tea.Cmd) {
	if len(m.rows) == 0 {
		m.editingRemark = false
		m.editValue = ""
		return m, nil
	}
	remark := strings.TrimSpace(m.editValue)
	cfg := m.rows[m.selected].Config
	cfg.Remark = remark
	if err := m.saveConfig(&cfg); err != nil {
		m.message = fmt.Sprintf("Save remark failed: %v", err)
		return m, nil
	}
	m.rows[m.selected].Config = cfg
	m.rows[m.selected].Remark = remark
	m.editingRemark = false
	m.editValue = ""
	m.message = fmt.Sprintf("Updated remark for %s", m.rows[m.selected].Key)
	return m, nil
}

func (m Model) markChecking() Model {
	for i := range m.rows {
		m.rows[i].Status = StatusChecking
	}
	m.message = ""
	return m
}

func (m Model) startStatusRefresh() tea.Cmd {
	return checkStatusesCmd(m.rows, defaultStatusWorkers, defaultStatusTimeout)
}

func (m Model) deleteSelected() (Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	key := m.rows[m.selected].Key
	if err := m.deleteConfig(key); err != nil {
		m.message = fmt.Sprintf("Delete failed: %v", err)
		return m, nil
	}
	m.rows = append(m.rows[:m.selected], m.rows[m.selected+1:]...)
	if m.selected >= len(m.rows) && m.selected > 0 {
		m.selected--
	}
	m.message = fmt.Sprintf("Deleted %s", key)
	return m, nil
}

func (m Model) statusCounts() (online, offline int) {
	for _, row := range m.rows {
		switch row.Status {
		case StatusOnline:
			online++
		case StatusOffline:
			offline++
		}
	}
	return online, offline
}

func Run() (*config.SSHConfig, error) {
	configs, err := sshconfig.List()
	if err != nil {
		return nil, err
	}
	model := NewModel(configs, sshconfig.Delete)
	program := tea.NewProgram(model)
	result, err := program.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := result.(Model)
	if !ok || finalModel.SelectedConfig == nil {
		return nil, nil
	}
	return finalModel.SelectedConfig, nil
}
