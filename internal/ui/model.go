package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rela/qiniu-logs/internal/qiniu"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginLeft(2)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			MarginLeft(2)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true).
			MarginLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginLeft(2).
			MarginTop(1)
)

type state int

const (
	stateLoading state = iota
	stateList
	stateDownloading
	stateDone
	stateError
)

type fileItem struct {
	info qiniu.FileInfo
}

func (i fileItem) Title() string {
	return filepath.Base(i.info.Key)
}

func (i fileItem) Description() string {
	return fmt.Sprintf("%s | %s | %s",
		qiniu.FormatSize(i.info.Size),
		i.info.PutTime.Format("2006-01-02 15:04:05"),
		i.info.Key,
	)
}

func (i fileItem) FilterValue() string {
	return i.info.Key
}

type Model struct {
	client   *qiniu.Client
	userID   string
	destDir  string
	state    state
	spinner  spinner.Model
	list     list.Model
	progress progress.Model
	files    []qiniu.FileInfo
	selected *qiniu.FileInfo
	err      error
	message  string
	downloaded int64
	total      int64
	width    int
	height   int
}

type filesLoadedMsg struct {
	files []qiniu.FileInfo
}

type downloadProgressMsg struct {
	downloaded int64
	total      int64
}

type downloadCompleteMsg struct {
	path string
}

type errMsg struct {
	err error
}

func NewModel(client *qiniu.Client, userID string, destDir string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(progress.WithDefaultGradient())

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("170")).
		BorderLeftForeground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("241")).
		BorderLeftForeground(lipgloss.Color("170"))

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = fmt.Sprintf("用户 %s 的日志文件", userID)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	return Model{
		client:  client,
		userID:  userID,
		destDir: destDir,
		state:   stateLoading,
		spinner: s,
		list:    l,
		progress: p,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadFiles,
	)
}

func (m Model) loadFiles() tea.Msg {
	files, err := m.client.ListFiles(context.Background(), m.userID, 0)
	if err != nil {
		return errMsg{err: err}
	}
	return filesLoadedMsg{files: files}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state != stateDownloading {
				return m, tea.Quit
			}
		case "enter":
			if m.state == stateList {
				selected, ok := m.list.SelectedItem().(fileItem)
				if ok {
					m.selected = &selected.info
					m.state = stateDownloading
					return m, tea.Batch(
						m.spinner.Tick,
						m.downloadFile,
					)
				}
			}
			if m.state == stateDone || m.state == stateError {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.progress.Width = msg.Width - 10

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case filesLoadedMsg:
		m.files = msg.files
		if len(msg.files) == 0 {
			m.state = stateError
			m.err = fmt.Errorf("未找到用户 %s 的日志文件", m.userID)
			return m, nil
		}

		items := make([]list.Item, len(msg.files))
		for i, f := range msg.files {
			items[i] = fileItem{info: f}
		}
		m.list.SetItems(items)
		m.state = stateList
		return m, nil

	case downloadProgressMsg:
		m.downloaded = msg.downloaded
		m.total = msg.total
		if m.total > 0 {
			return m, nil
		}

	case downloadCompleteMsg:
		m.state = stateDone
		m.message = msg.path
		return m, nil

	case errMsg:
		m.state = stateError
		m.err = msg.err
		return m, nil
	}

	if m.state == stateList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) downloadFile() tea.Msg {
	if m.selected == nil {
		return errMsg{err: fmt.Errorf("未选择文件")}
	}

	filename := filepath.Base(m.selected.Key)
	destPath := filepath.Join(m.destDir, filename)

	home, _ := os.UserHomeDir()
	if strings.HasPrefix(destPath, home) {
		destPath = filepath.Join(m.destDir, filename)
	}

	progressChan := make(chan downloadProgressMsg, 100)
	doneChan := make(chan error, 1)

	go func() {
		err := m.client.DownloadFile(
			context.Background(),
			m.selected.Key,
			destPath,
			func(downloaded, total int64) {
				select {
				case progressChan <- downloadProgressMsg{downloaded: downloaded, total: total}:
				default:
				}
			},
		)
		doneChan <- err
	}()

	go func() {
		for range progressChan {
		}
	}()

	if err := <-doneChan; err != nil {
		return errMsg{err: err}
	}

	return downloadCompleteMsg{path: destPath}
}

func (m Model) View() string {
	var s strings.Builder

	switch m.state {
	case stateLoading:
		s.WriteString(fmt.Sprintf("\n  %s 正在加载用户 %s 的日志文件...\n", m.spinner.View(), m.userID))

	case stateList:
		s.WriteString(m.list.View())
		s.WriteString(helpStyle.Render("\n  ↑/↓: 选择 • /: 搜索 • Enter: 下载 • q: 退出"))

	case stateDownloading:
		s.WriteString(fmt.Sprintf("\n  %s 正在下载 %s...\n\n", m.spinner.View(), filepath.Base(m.selected.Key)))
		if m.total > 0 {
			percent := float64(m.downloaded) / float64(m.total)
			s.WriteString(fmt.Sprintf("  %s\n", m.progress.ViewAs(percent)))
			s.WriteString(fmt.Sprintf("\n  %s / %s", qiniu.FormatSize(m.downloaded), qiniu.FormatSize(m.total)))
		}

	case stateDone:
		s.WriteString(successStyle.Render("\n✓ 下载完成!\n\n"))
		s.WriteString(statusStyle.Render(fmt.Sprintf("  文件已保存到: %s\n", m.message)))
		s.WriteString(helpStyle.Render("\n  按 Enter 或 q 退出"))

	case stateError:
		s.WriteString(errorStyle.Render(fmt.Sprintf("\n✗ 错误: %s\n", m.err.Error())))
		s.WriteString(helpStyle.Render("\n  按 Enter 或 q 退出"))
	}

	return s.String()
}
