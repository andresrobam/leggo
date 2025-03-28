package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andresrobam/leggo/config"
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

func saveContextSettings() {
	serviceOrder := make([]string, len(services))
	for i := range services {
		serviceOrder[i] = services[i].Key
	}
	context.Settings.ServiceOrder = serviceOrder
	context.Settings.ActiveService = services[activeIndex].Key
	if err := config.WriteContextSettings(&context.FilePath, &context.Settings); err != nil {
		// TODO: show error modal
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

	saveContextSettings()
	activeMutex.Unlock()
}

func swap(increment int) {
	if len(services) < 2 {
		return
	}
	activeMutex.Lock()
	defer activeMutex.Unlock()
	newActiveIndex := activeIndex + increment

	pushAllElements := 0
	if newActiveIndex < 0 {
		newActiveIndex = len(services) - 1
		pushAllElements = -1
	} else if newActiveIndex >= len(services) {
		newActiveIndex = 0
		pushAllElements = 1
	}
	if pushAllElements == 0 {
		active := services[activeIndex]
		services[activeIndex] = services[newActiveIndex]
		services[newActiveIndex] = active
	} else {
		newServicesOrder := make([]*service.Service, len(services))
		for i := range services {
			if i == activeIndex {
				continue
			}
			newServicesOrder[i+pushAllElements] = services[i]
		}
		newServicesOrder[newActiveIndex] = services[activeIndex]
		services = newServicesOrder
	}
	activeIndex = newActiveIndex
	saveContextSettings()
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
		} else if k == "enter" || k == "space" {
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
		contextStyle.Render(context.Name),
		runningCountStyle.Render(fmt.Sprintf("%d/%d running", runningServiceCount(), len(services))),
		pid,
		logSizeStyle.Render(fmt.Sprintf("Log: %s", formatDataSize(len(services[activeIndex].Content)))),
		scrollStyle.Render(strconv.FormatInt(int64(m.log.ScrollPercent()), 10)+"%"))
}

var context *Context
var services []*service.Service
var activeIndex = 0
var quitting bool
var configuration config.Config

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

type Context struct {
	Name     string
	Settings config.ContextSettings
	FilePath string
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

	var contextDefinition contextDefinition

	if err := yaml.ImportYaml(ymlData, &contextDefinition); err != nil {
		fmt.Println("Error reading yaml: ", err)
		os.Exit(1)
	}
	if len(contextDefinition.Services) == 0 {
		fmt.Println("No services defined, must define at least 1 service.")
		os.Exit(1)
	}
	// TODO: yaml validataion

	context = &Context{}
	if contextDefinition.Name == "" {
		context.Name = yaml.WithoutExtension(fileName)
	} else {
		context.Name = contextDefinition.Name
	}
	absoluteFilePath, err := filepath.Abs(fileName)
	context.FilePath = absoluteFilePath
	if err != nil {
		fmt.Println("Error getting absolute path of file: ", err)
		os.Exit(1)
	}

	contextSettingsMap := make(map[string]config.ContextSettings)
	if config.ReadContextSettings(&contextSettingsMap) != nil {
		context.Settings = config.ContextSettings{}
	} else if contextSettings, ok := contextSettingsMap[context.FilePath]; ok {
		context.Settings = contextSettings
	} else {
		context.Settings = config.ContextSettings{}
	}
	if context.Settings.ServiceOrder == nil {
		context.Settings.ServiceOrder = make([]string, 0)
	}

	existingServiceKeys, _ := yaml.GetKeys(ymlData, "$.services")

	serviceIndex := 0
	finalServiceKeys := make([]string, len(existingServiceKeys))

	for _, serviceKey := range context.Settings.ServiceOrder {
		if slices.Contains(existingServiceKeys, serviceKey) {
			finalServiceKeys[serviceIndex] = serviceKey
			serviceIndex++
		}
	}

	if serviceIndex != len(existingServiceKeys) {
		for _, serviceKey := range existingServiceKeys {
			if !slices.Contains(finalServiceKeys, serviceKey) {
				finalServiceKeys[serviceIndex] = serviceKey
				serviceIndex++
			}
		}
	}

	services = make([]*service.Service, len(finalServiceKeys))
	for i, serviceKey := range finalServiceKeys {
		var name string
		s := contextDefinition.Services[serviceKey]
		if s.Name != "" {
			name = s.Name
		} else {
			name = serviceKey
		}
		newService := service.New(serviceKey, name, s.Path, s.Commands)
		services[i] = &newService
	}

	if context.Settings.ActiveService != "" {
		if savedActiveServiceIndex := slices.Index(finalServiceKeys, context.Settings.ActiveService); savedActiveServiceIndex != -1 {
			activeIndex = savedActiveServiceIndex
		}
	}
	services[activeIndex].Active = true

	configuration = config.Config{}
	config.ApplyDefaults(&configuration)

	if err := config.ReadConfig(&configuration); err != nil {
		configuration = config.Config{}
		config.ApplyDefaults(&configuration)
	}

	p := tea.NewProgram(
		model{},
		tea.WithAltScreen(),
		tea.WithKeyboardEnhancements(tea.WithKeyReleases),
	)

	for i := range services {
		services[i].Program = p
		services[i].Configuration = &configuration
	}

	go func() {
		ticker := time.NewTicker(time.Duration(configuration.RefreshMillis) * time.Millisecond)
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

// TODO: more splitting of functions and modules and files and shit
// TODO: ansi hardwrap for lines
// TODO: separate methods for adding lines to log vs rerendering on active change/window size change
// TODO: ability to grep logs
// TODO: pretty status bar
// TODO: pretty header
// TODO: handle too many elements on header for viewport width
// TODO: handel too many elements on footer for viewport width
// TODO: better keyboard controls
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to std messages
// TODO: remember timestamp rules per context service
// TODO: style sysout messages
// TODO: style syserr messages
// TODO: allow relative paths in context yml files
// TODO: system to make sure some services arent started in parallel
// TODO: requirements (one service can depend on another)
// TODO: healthchecks (that make sure requirements are complete)
// TODO: allow overriding success codes for commands
// TODO: add optional context name override param
// TODO: show quitting status somewhere
