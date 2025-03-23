package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andresrobam/leggo/log"
	"github.com/andresrobam/leggo/service"
	"github.com/andresrobam/leggo/yaml"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type model struct {
	ready bool
	log   log.Log
}

func (m model) Init() (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *model) setViewportContent(s *service.Service) {
	goToBottom := m.log.AtBottom()
	m.log.SetContent(s.Content)
	if goToBottom {
		m.log.GotoBottom()
	}
}

func changeActive(m *model, increment int) {
	activeMutex.Lock()
	if len(services) < 2 {
		return
	}
	services[activeIndex].YOffset = m.log.YOffset
	services[activeIndex].WasAtBottom = m.log.AtBottom()
	services[activeIndex].Active = false
	activeIndex += increment
	if activeIndex < 0 {
		activeIndex = len(services) - 1
	} else if activeIndex >= len(services) {
		activeIndex = 0
	}
	services[activeIndex].Active = true
	m.setViewportContent(services[activeIndex])
	if services[activeIndex].WasAtBottom {
		m.log.GotoBottom()
	} else {
		m.log.SetYOffset(services[activeIndex].YOffset)
	}
	activeMutex.Unlock()
}

func swap(increment int) {
	// TODO: don't swap if going over the bounds, push others aside instead
	if len(services) < 2 {
		return
	}
	activeMutex.Lock()
	defer activeMutex.Unlock()
	newActiveIndex := activeIndex + increment
	if newActiveIndex < 0 {
		newActiveIndex = len(services) - 1
	} else if newActiveIndex >= len(services) {
		newActiveIndex = 0
	}
	active := services[activeIndex]
	services[activeIndex] = services[newActiveIndex]
	services[newActiveIndex] = active
	activeIndex = newActiveIndex
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
		} else if k := msg.String(); k == "ctrl+c" || k == "q" || k == "esc" {
			quitting = true
			var anyRunning bool
			for i := range services {
				services[i].StateMutex.Lock()
				if services[i].State != service.StateStopped {
					anyRunning = true
					services[i].EndService()
				}
				services[i].StateMutex.Unlock()
			}
			if !anyRunning {
				return m, tea.Quit
			}
		} else if k == "w" {
			m.log.GotoBottom()
		} else if k == "enter" {
			activeMutex.RLock()
			services[activeIndex].StateMutex.Lock()
			switch services[activeIndex].State {
			case service.StateStopped:
				if !quitting {
					services[activeIndex].StartService()
				}
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
		} else if k == "shift+left" || k == "shift+h" {
			swap(-1)
		} else if k == "shift+right" || k == "shift+l" {
			swap(1)
		} else if k == "up" || k == "k" {
			m.log.LineUp(1)
		} else if k == "down" || k == "j" {
			m.log.LineDown(1)
		}
		/*
			// TODO: move to log
			   func (m Model) updateAsModel(msg tea.Msg) Model {
			   	if !m.initialized {
			   		m.setInitialValues()
			   	}

			   	switch msg := msg.(type) {
			   	case tea.KeyPressMsg:
			   		switch {
			   		case key.Matches(msg, m.KeyMap.PageDown):
			   			m.ViewDown()

			   		case key.Matches(msg, m.KeyMap.PageUp):
			   			m.ViewUp()

			   		case key.Matches(msg, m.KeyMap.HalfPageDown):
			   			m.HalfViewDown()

			   		case key.Matches(msg, m.KeyMap.HalfPageUp):
			   			m.HalfViewUp()

			   		case key.Matches(msg, m.KeyMap.Down):
			   			m.LineDown(1)

			   		case key.Matches(msg, m.KeyMap.Up):
			   			m.LineUp(1)
			   		}

			   	return m
			   }*/

	case tea.MouseWheelMsg:
		if !m.log.MouseWheelEnabled {
			break
		}

		switch msg.Button { //nolint:exhaustive
		case tea.MouseWheelDown:
			m.log.LineDown(m.log.MouseWheelDelta)

		case tea.MouseWheelUp:
			m.log.LineUp(m.log.MouseWheelDelta)
		}

	case service.ContentUpdateMsg:
		activeMutex.RLock()
		if services[activeIndex].ContentUpdated.Swap(false) {
			m.setViewportContent(services[activeIndex])
		}
		activeMutex.RUnlock()

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
			m.log = log.New()
			m.log.SetWidth(msg.Width)
			m.log.SetHeight(msg.Height - verticalMarginHeight)
			m.ready = true
		} else {
			m.log.SetWidth(msg.Width)
			m.log.SetHeight(msg.Height - verticalMarginHeight)
			m.setViewportContent(services[activeIndex])
		}
	}

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.log.View(), m.footerView())
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

var contextName string

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
var logSizeStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
var scrollStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
var pidStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

const BYTE_MULTIPLIER float32 = 1024
const UNITS string = "BKMGTPE"

func formatDataSize(bytes int) string {
	var unitIndex int
	size := float32(bytes)
	for {
		new_size := size / BYTE_MULTIPLIER
		if new_size < 1 {
			break
		}
		size = new_size
		unitIndex++
		if unitIndex == len(UNITS)-1 {
			break
		}
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", size), "0"), ".") + string(UNITS[unitIndex])
}

func (m model) footerView() string {

	var pid string
	if services[activeIndex].Pid != 0 {
		pid = pidStyle.Render(fmt.Sprintf("PID: %d", services[activeIndex].Pid))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		contextStyle.Render(contextName),
		runningCountStyle.Render(fmt.Sprintf("%d/%d running", runningServiceCount(), len(services))),
		pid,
		logSizeStyle.Render(fmt.Sprintf("Log: %s", formatDataSize(len(services[activeIndex].Content)))),
		scrollStyle.Render(strconv.FormatInt(int64(m.log.ScrollPercent()), 10)+"%"))
}

var services []*service.Service
var activeIndex = 0
var quitting bool

var activeMutex sync.RWMutex

type contextDefinition struct {
	Name     string
	Services map[string]struct {
		Name     string
		Path     string
		Commands []string
		Requires []string
	}
}

func main() {

	if len(os.Args) < 2 {
		fmt.Println("No file name provided.")
		os.Exit(1)
	}

	fileName := os.Args[1]

	ymlData, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Println("Error opening file: ", err)
		os.Exit(1)
	}

	var context contextDefinition

	if err := yaml.ImportYaml(ymlData, context); err != nil {
		fmt.Println("Error reading yaml: ", err)
		os.Exit(1)
	}
	// TODO: yaml validataion
	servicesKeys, _ := yaml.GetKeys(ymlData, "$.services")

	services = make([]*service.Service, len(servicesKeys))
	for i := range servicesKeys {
		var name string
		s := context.Services[servicesKeys[i]]
		if s.Name != "" {
			name = s.Name
		} else {
			name = servicesKeys[i]
		}
		newService := service.New(name, s.Path, s.Commands)
		services[i] = &newService
	}

	services[activeIndex].Active = true

	if context.Name == "" {
		contextName = yaml.WithoutExtension(fileName)
	} else {
		contextName = context.Name
	}

	p := tea.NewProgram(
		model{},
		tea.WithAltScreen(),
		tea.WithKeyboardEnhancements(tea.WithKeyReleases),
	)

	for i := range services {
		services[i].Program = p
	}

	go func() {
		ticker := time.NewTicker(6 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			p.Send(service.ContentUpdateMsg{})
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running bubbletea program: ", err)
		os.Exit(1)
	}
}

// TODO: ansi hardwrap for lines
// TODO: separate methods for adding lines to log vs rerendring on active change/window size change
// TODO: pretty status bar
// TODO: pretty header
// TODO: better keyboard controls
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to std messages
// TODO: remember timestamp rules per id
// TODO: style sysout messages
// TODO: style syserr messages
// TODO: relative paths
// TODO: system to make sure some services arent started in parallel
// TODO: requirements (one service can depend on another)
// TODO: healthchecks (that make sure requirements are complete)
// TODO: allow overriding success codes for commands
// TODO: add optional context name param
// TODO: save custom order to ~/.config/leggo.yml on every order switch
// TODO: save active service name to ~/.config/leggo.yml on every active tab switch
// TODO: show quitting status somewhere
// TODO: popup for non-service errors
// TODO: if context specific conf exists and has custom order, apply on load (delete old non-existing services from config and add new services to the end of the list)
// TODO: if context specific conf exists and active tab, apply on load (default to 0 if missing or out of range)
