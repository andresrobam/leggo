package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/andresrobam/leggo/service"
	"github.com/charmbracelet/bubbles/v2/viewport"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type model struct {
	ready    bool
	viewport viewport.Model
	follow   bool
}

func (m model) Init() (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *model) setViewportContent(s *service.Service) {
	var content string
	s.LineMutex.RLock()
	for _, line := range s.Lines {
		content += line.Text + "\n"
	}
	s.LineMutex.RUnlock()
	m.viewport.SetContent(content)
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func changeActive(m *model, increment int) {
	activeMutex.Lock()
	if len(services) < 2 {
		return
	}
	services[activeIndex].Active = false
	activeIndex += increment
	if activeIndex < 0 {
		activeIndex = len(services) - 1
	} else if activeIndex >= len(services) {
		activeIndex = 0
	}
	services[activeIndex].Active = true
	m.setViewportContent(&services[activeIndex])
	activeMutex.Unlock()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.IsRepeat {
			break
		} else if k := msg.String(); (k == "ctrl+c" || k == "q" || k == "esc") && !quitting {
			// TODO: send again to running/stopping services if pressed a second time
			quitting = true
			var anyRunning bool
			for i := range services {
				services[i].StateMutex.Lock()
				if services[i].State == service.StateRunning {
					anyRunning = true
					services[i].EndService()
				}
				services[i].StateMutex.Unlock()
			}
			if !anyRunning {
				return m, tea.Quit
			}
		} else if k == "w" {
			if !m.follow {
				m.viewport.GotoBottom()
			}
			m.follow = !m.follow
		} else if k == "enter" && !quitting {
			activeMutex.RLock()
			services[activeIndex].StateMutex.Lock()
			switch services[activeIndex].State {
			case service.StateStopped:
				services[activeIndex].StartService()
			case service.StateRunning:
				services[activeIndex].EndService()
			case service.StateStopping:
				services[activeIndex].EndService()
			}
			services[activeIndex].StateMutex.Unlock()
			activeMutex.RUnlock()
		} else if k == "left" || k == "h" {
			changeActive(&m, -1)
		} else if k == "right" || k == "l" {
			changeActive(&m, 1)
		}

	case service.ContentUpdateMsg:
		m.setViewportContent(&services[activeIndex])

	case service.ServiceStoppedMsg:
		if quitting {
			var anyRunning bool
			for i := range services {
				services[i].StateMutex.RLock()
				if services[i].State != service.StateStopped {
					anyRunning = true
					services[i].StateMutex.RUnlock()
					break
				}
				services[i].StateMutex.RUnlock()
			}
			if !anyRunning {
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(msg.Height - verticalMarginHeight)
			//m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(msg.Height - verticalMarginHeight)
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

var cmdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("63"))

var activeCmdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("9"))

var stoppedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(9))
var runningStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(10))
var stoppingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(11))

func (m model) headerView() string {

	titles := make([]string, len(services))
	for i := range services {
		var tabStyle *lipgloss.Style
		var stateStyle *lipgloss.Style
		services[i].StateMutex.RLock()
		switch services[i].State {
		case service.StateRunning:
			stateStyle = &runningStyle
		case service.StateStopping:
			stateStyle = &stoppingStyle
		default:
			stateStyle = &stoppedStyle
		}
		services[i].StateMutex.RUnlock()
		if services[i].Active {
			tabStyle = &activeCmdStyle
		} else {
			tabStyle = &cmdStyle
		}
		titles[i] = tabStyle.Render(stateStyle.Render("● ")+services[i].Name) + "\n"
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, titles...)
}

const contextName = "brew"

func runningServiceCount() (count int) {
	for i := range services {
		switch services[i].State {
		case service.StateRunning:
			count++
		case service.StateStopping:
			count++
		}
	}
	return
}

var contextStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
var runningCountStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
// TODO: K/B/M/G/T/P/E function
func (m model) footerView() string {
	context := contextStyle.Render(contextName)
	running := runningCountStyle.Render(fmt.Sprintf("%d/%d running", runningServiceCount(), len(services)))
	log := fmt.Sprintf("Log: %d lines / %d B", len(services[activeIndex].Lines), services[activeIndex].ContentBytes)
	scroll := fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)

	components := []string{context, running, log, scroll}
	return lipgloss.JoinHorizontal(lipgloss.Center, components...)
}

var services = make([]service.Service, 0)
var activeIndex = 0
var quitting bool

var activeMutex sync.RWMutex

func main() {

	p := tea.NewProgram(
		model{follow: true},
		tea.WithAltScreen(),
		tea.WithKeyboardEnhancements(tea.WithKeyReleases),
	)

	services = append(services, service.Service{Program: p, Name: "UI", Path: "C:\\Users\\Andres\\workspace\\brew\\ui", Lines: make([]service.Line, 0), Commands: []string{"bun install", "bun run dev"}})
	services = append(services, service.Service{Program: p, Name: "docker w ansi", Path: "C:\\Users\\Andres\\workspace\\brew", Lines: make([]service.Line, 0), Commands: []string{"docker compose --ansi=auto up"}})
	services = append(services, service.Service{Program: p, Name: "docker wo ansi", Path: "C:\\Users\\Andres\\workspace\\brew", Lines: make([]service.Line, 0), Commands: []string{"docker compose up"}})
	services[activeIndex].Active = true

	if _, err := p.Run(); err != nil {
		fmt.Println("could not run program:", err)
		os.Exit(1)
	}
}

// TODO: when ending command and when lines has no stdout/sterr lines, send end event again in 5s

// TODO: status
// TODO: header
// TODO: OS specific command execution
// TODO: OS specific command stop
// TODO: OS specific multi command support
// TODO: per-cr follow/scroll state
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to stdout/stderr messages
// TODO: get services from file
// TODO: contexts
// TODO: requirements
// TODO: healthchecks
// TODO: remember timestamp rules per id
// TODO: config file
//       command executor
//       command splitter
//       docker compose ansi
//       max string length for command output
//       allow breaking u p strings during byte thing
