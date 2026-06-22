package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/wuxs/ssm/pkg/config"
)

var (
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	selectedRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62"))
	onlineStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	offlineStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	checkingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	unknownStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type ConnectionStatus string

const (
	StatusUnknown  ConnectionStatus = "unknown"
	StatusChecking ConnectionStatus = "checking"
	StatusOnline   ConnectionStatus = "online"
	StatusOffline  ConnectionStatus = "offline"
)

type ConnectionRow struct {
	Config      config.SSHConfig
	Remark      string
	Key         string
	Address     string
	Status      ConnectionStatus
	LastUsed    string
	HasPassword string
	HasKey      string
	Auth        string
}

func newConnectionRow(cfg config.SSHConfig) ConnectionRow {
	key := cfg.GetKey()
	remark := cfg.Remark
	if remark == "" && cfg.Alias != "" && cfg.Alias != cfg.Host {
		remark = cfg.Alias
	}
	return ConnectionRow{
		Config:      cfg,
		Remark:      remark,
		Key:         key,
		Address:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Status:      StatusUnknown,
		LastUsed:    formatLastUsed(cfg.LastUsed),
		HasPassword: yesNo(cfg.Password != ""),
		HasKey:      yesNo(cfg.PrivateKey != ""),
		Auth:        cfg.GetAuthDescription(),
	}
}

func rowsFromConfigs(configs []config.SSHConfig) []ConnectionRow {
	rows := make([]ConnectionRow, 0, len(configs))
	for _, cfg := range configs {
		rows = append(rows, newConnectionRow(cfg))
	}
	return rows
}

func formatLastUsed(lastUsed string) string {
	if lastUsed == "" {
		return "never"
	}
	parsed, err := time.Parse(time.RFC3339, lastUsed)
	if err != nil {
		return lastUsed
	}
	return parsed.Format("2006-01-02 15:04")
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func renderRows(rows []ConnectionRow, selected int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-2s%-20s %-32s %-10s %-16s %-5s %-5s %s\n", "", "Remark", "Host", "Status", "Last Used", "Pwd", "Key", "Auth"))
	for i, row := range rows {
		cursor := ""
		if i == selected {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%-2s%-20s %-32s %-10s %-16s %-5s %-5s %s\n",
			cursor,
			truncate(row.Remark, 20),
			truncate(row.Key, 32),
			row.Status,
			truncate(row.LastUsed, 16),
			row.HasPassword,
			row.HasKey,
			row.Auth,
		))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderRowsStyled(rows []ConnectionRow, selected int) string {
	var b strings.Builder
	b.WriteString(tableHeaderStyle.Render(fmt.Sprintf("%-2s%-20s %-32s %-10s %-16s %-5s %-5s %s", "", "Remark", "Host", "Status", "Last Used", "Pwd", "Key", "Auth")))
	for i, row := range rows {
		cursor := ""
		if i == selected {
			cursor = ">"
		}
		line := renderStyledRow(cursor, row)
		if i == selected {
			line = selectedRowStyle.Render(line)
		}
		b.WriteString("\n")
		b.WriteString(line)
	}
	return b.String()
}

func renderStyledRow(cursor string, row ConnectionRow) string {
	return fmt.Sprintf("%-2s%s %s %s %s %s %s %s",
		cursor,
		padCell(truncate(row.Remark, 20), 20),
		padCell(truncate(row.Key, 32), 32),
		styleStatus(row.Status, 10),
		padCell(truncate(row.LastUsed, 16), 16),
		padCell(row.HasPassword, 5),
		padCell(row.HasKey, 5),
		row.Auth,
	)
}

func styleStatus(status ConnectionStatus, width int) string {
	value := padCell(string(status), width)
	switch status {
	case StatusOnline:
		return onlineStyle.Render(value)
	case StatusOffline:
		return offlineStyle.Render(value)
	case StatusChecking:
		return checkingStyle.Render(value)
	default:
		return unknownStyle.Render(value)
	}
}

func padCell(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}
