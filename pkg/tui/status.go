package tui

import (
	"net"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	defaultStatusTimeout = time.Second
	defaultStatusWorkers = 8
)

type statusBatchMsg struct {
	Results map[string]ConnectionStatus
}

func checkTCPStatus(address string, timeout time.Duration) ConnectionStatus {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return StatusOffline
	}
	_ = conn.Close()
	return StatusOnline
}

func checkStatusesCmd(rows []ConnectionRow, workers int, timeout time.Duration) tea.Cmd {
	jobs := make([]ConnectionRow, len(rows))
	copy(jobs, rows)
	if workers <= 0 {
		workers = defaultStatusWorkers
	}
	if timeout <= 0 {
		timeout = defaultStatusTimeout
	}

	return func() tea.Msg {
		results := make(map[string]ConnectionStatus, len(jobs))
		jobCh := make(chan ConnectionRow)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for row := range jobCh {
					status := checkTCPStatus(row.Address, timeout)
					mu.Lock()
					results[row.Key] = status
					mu.Unlock()
				}
			}()
		}

		for _, row := range jobs {
			jobCh <- row
		}
		close(jobCh)
		wg.Wait()

		return statusBatchMsg{Results: results}
	}
}
