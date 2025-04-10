package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/andresrobam/leggo/config"
	"github.com/andresrobam/leggo/service"
	"github.com/andresrobam/leggo/yaml"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type model struct {
	ready bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func saveContextSettings() {
	serviceOrder := make([]string, len(services))
	for i := range services {
		serviceOrder[i] = services[i].Key
	}
	context.Settings.ServiceOrder = serviceOrder
	context.Settings.ActiveService = activeService.Key

	if context.Settings.ActiveService != "" && context.Settings.ActiveService != activeService.Key {

	}
	if err := config.WriteContextSettings(&context.FilePath, &context.Settings); err != nil {
		// TODO: show error modal
	}
}

func changeActive(increment int) {
	if len(services) < 2 {
		return
	}
	activeMutex.Lock()
	activeIndex += increment
	if activeIndex < 0 {
		activeIndex = len(services) - 1
	} else if activeIndex >= len(services) {
		activeIndex = 0
	}
	activeService = services[activeIndex]
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
		} else if k == "enter" || k == "space" {
			activeMutex.RLock()
			activeService.StateMutex.Lock()
			switch activeService.State {
			case service.StateStopped:
				if !quitting {
					activeService.StartService()
				}
			case service.StateRunning:
				activeService.EndService()
			case service.StateStopping:
				activeService.EndService()
			}
			activeService.StateMutex.Unlock()
			activeMutex.RUnlock()
		} else if k == "left" || k == "h" {
			changeActive(-1)
		} else if k == "right" || k == "l" {
			changeActive(1)
		} else if k == "shift+left" || k == "shift+h" {
			swap(-1)
		} else if k == "shift+right" || k == "shift+l" {
			swap(1)
		} else if k == "up" || k == "k" {
			activeMutex.RLock()
			activeService.Log.Scroll(-1)
			activeMutex.RUnlock()
		} else if k == "down" || k == "j" {
			activeMutex.RLock()
			activeService.Log.Scroll(1)
			activeMutex.RUnlock()
		} else if k == "b" || k == "f" {
			activeService.Log.GotoBottom()
		}

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

		activeMutex.RLock()

		for i := range services {
			services[i].Log.SetSize(msg.Width, msg.Height-headerHeight-footerHeight)
		}

		if !m.ready {
			m.ready = true
		}

		activeMutex.RUnlock()
	}

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}
	activeMutex.RLock()
	defer activeMutex.RUnlock()
	logView, clearScreen := activeService.Log.View()
	if clearScreen {
		//go p.Send(tea.ClearScreen())
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), logView, m.footerView())
}

var cmdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("63"))

var activeCmdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("9"))

var stoppedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ff0000"))
var runningStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#00ff00"))
var stoppingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffff00"))

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
		if i == activeIndex {
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

const statusBarTextColor = "#dddddd"

const statusBarColor0 = "#12afe3"
const statusBarColor1 = "#128ce3"
const statusBarColor2 = "#1262e3"
const statusBarColor3 = "#123ce3"

var contextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarTextColor)).Background(lipgloss.Color(statusBarColor0))
var contextTransitionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarColor0)).Background(lipgloss.Color(statusBarColor1))

var runningCountStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarTextColor)).Background(lipgloss.Color(statusBarColor1))
var runningCountTransitionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarColor1)).Background(lipgloss.Color(statusBarColor2))

var logSizeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarTextColor)).Background(lipgloss.Color(statusBarColor2))
var logSizeTransitionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarColor2)).Background(lipgloss.Color(statusBarColor3))

var pidStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarTextColor)).Background(lipgloss.Color(statusBarColor3))
var pidTransitionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarColor3))

func (m model) footerView() string {

	var pid string
	if activeService.Pid != 0 {
		pid = pidStyle.Render(fmt.Sprintf(" PID %d ", activeService.Pid))
	} else {
		pid = pidStyle.Render(" PID - ")
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		contextStyle.Render(" "+context.Name+" "),
		contextTransitionStyle.Render("\uE0B0"),
		runningCountStyle.Render(fmt.Sprintf(" %d/%d running ", runningServiceCount(), len(services))),
		runningCountTransitionStyle.Render("\uE0B0"),
		logSizeStyle.Render(fmt.Sprintf(" Log: %s ", formatDataSize(activeService.Log.GetContentSize()))),
		logSizeTransitionStyle.Render("\uE0B0"),
		pid,
		pidTransitionStyle.Render("\uE0B0"),
	)
}

// TODO: all of the status bar style stuff could be a for loop

var context *Context
var services []*service.Service
var activeService *service.Service
var activeIndex = 0
var quitting bool
var configuration config.Config

var activeMutex sync.RWMutex

type contextDefinition struct {
	Name     string `yaml:"name"`
	Services map[string]struct {
		Name     string
		Path     string
		Commands []string
		Requires []string
	} `yaml:"services"`
}

type Context struct {
	Name     string
	Settings config.ContextSettings
	FilePath string
}

var p *tea.Program

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

	var configuration config.Config
	config.ApplyDefaults(&configuration)

	if err := config.ReadConfig(&configuration); err != nil {
		configuration = config.Config{}
		config.ApplyDefaults(&configuration)
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

	contextDir := filepath.Dir(absoluteFilePath)
	services = make([]*service.Service, len(finalServiceKeys))
	for i, serviceKey := range finalServiceKeys {
		var name string
		s := contextDefinition.Services[serviceKey]
		if s.Name != "" {
			name = s.Name
		} else {
			name = serviceKey
		}

		servicePath := s.Path
		if servicePath == "" {
			servicePath = contextDir
		} else if !filepath.IsAbs(servicePath) {
			servicePath, _ = filepath.Abs(filepath.Join(contextDir, servicePath))
		}

		newService := service.New(serviceKey, name, servicePath, s.Commands, &configuration)
		services[i] = &newService
	}

	if context.Settings.ActiveService != "" {
		if savedActiveServiceIndex := slices.Index(finalServiceKeys, context.Settings.ActiveService); savedActiveServiceIndex != -1 {
			activeIndex = savedActiveServiceIndex
		}
	}
	activeService = services[activeIndex]

	p = tea.NewProgram(
		model{},
		tea.WithAltScreen(),
		tea.WithKeyboardEnhancements(tea.WithKeyReleases),
	)
	for i := range services {
		services[i].Program = p
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
// TODO: ability to grep logs
// TODO: pretty header
// TODO: handle too many elements on header for viewport width
// TODO: handle too many elements on footer for viewport width
// TODO: better keyboard controls
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to std messages
// TODO: remember timestamp rules per context service
// TODO: style sysout messages
// TODO: style syserr messages
// TODO: system to make sure some services arent started in parallel
// TODO: requirements (one service can depend on another)
// TODO: healthchecks (that make sure requirements are complete)
// TODO: allow overriding success codes for commands
// TODO: add optional context name override param
// TODO: show quitting status somewhere
