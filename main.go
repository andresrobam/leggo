package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
	ready  bool
	width  int
	height int
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
		headerHeight := lipgloss.Height(m.headerView(msg.Width))
		footerHeight := lipgloss.Height(m.footerView(msg.Width))
		m.height = msg.Height
		m.width = msg.Width

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
	logView, _ := activeService.Log.View()
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(m.width), logView, m.footerView(m.width))
}

var cmdStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#cccccc")).
	Background(lipgloss.Color("#555555"))

var altCmdStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#cccccc")).
	Background(lipgloss.Color("#444444"))

var activeCmdStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffffff")).
	Background(lipgloss.Color("#3333dd"))

var stoppedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ff0000"))
var runningStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#00ff00"))
var stoppingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffff00"))

func (m model) headerView(width int) string {

	var title string
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
		} else if i%2 == 0 {
			tabStyle = &cmdStyle
		} else {
			tabStyle = &altCmdStyle
		}
		title += tabStyle.Render(" ") + stateStyle.Inherit(*tabStyle).Render("●") + tabStyle.Render(" "+services[i].Name+" ")
	}
	return lipgloss.NewStyle().Width(width).Render(title)
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

var statusBarTextColor = lipgloss.Color("#dddddd")

var statusBarBackgroundColors = []color.Color{
	lipgloss.Color("#12afe3"),
	lipgloss.Color("#128ce3"),
	lipgloss.Color("#1262e3"),
	lipgloss.Color("#123ce3"),
}

func (m model) footerView(width int) string {

	pid := "-"
	if activeService.Pid != 0 {
		pid = strconv.Itoa(activeService.Pid)
	}

	statusBarItems := []string{
		context.Name,
		fmt.Sprintf("%d/%d running", runningServiceCount(), len(services)),
		fmt.Sprintf("Log: %s", formatDataSize(activeService.Log.GetContentSize())),
		fmt.Sprintf("PID %s", pid),
	}

	renderItems := make([]string, len(statusBarItems)*2)

	for i, item := range statusBarItems {
		// var contextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarTextColor)).Background(lipgloss.Color(statusBarColor0))
		// var contextTransitionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusBarColor0)).Background(lipgloss.Color(statusBarColor1))
		renderItems[i*2] = lipgloss.NewStyle().
			Foreground(statusBarTextColor).
			Background(statusBarBackgroundColors[i]).
			Render(" " + item + " ")
		transitionStyle := lipgloss.NewStyle().
			Foreground(statusBarBackgroundColors[i])

		if i != len(statusBarItems)-1 {
			transitionStyle = transitionStyle.
				Background(statusBarBackgroundColors[i+1])
		}

		renderItems[i*2+1] = transitionStyle.Render("\uE0B0")
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, renderItems...)
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
		Commands []service.Command
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

	if runtime.GOOS == "windows" {
		os.Setenv("TEA_STANDARD_RENDERER", "true")
	}

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
// TODO: ability to search logs (case sensitive, case insensitive, regex)
// TODO: pretty header
// TODO: handle too many elements on footer for viewport width
// TODO: more keyboard controls (page up, page down, half page up, half page down)
// TODO: show keyboard commands somewhere
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to std messages
// TODO: remember timestamp rules per context service
// TODO: style sysout messages
// TODO: style syserr messages
// TODO: system to make sure some services arent started in parallel
// TODO: requirements (one service can depend on another)
// TODO: healthchecks (that make sure requirements are complete)
// TODO: allow overriding success codes for commands
// TODO: show quitting status somewhere
// TODO: filter to only show running tabs
// TODO: automatically send second stop after 30s and then every 5s after that
// TODO: make windows gradle/maven/java kill optional
// TODO: add kill options as regex to config
// TODO: add command replacement regex to config
