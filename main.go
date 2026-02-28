package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/andresrobam/leggo/config"
	"github.com/andresrobam/leggo/lock"
	"github.com/andresrobam/leggo/log"
	"github.com/andresrobam/leggo/service"
	"github.com/andresrobam/leggo/yaml"
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

func changeActive(right bool) {
	if len(services) < 2 {
		return
	}
	activeMutex.Lock()
	defer activeMutex.Unlock()

	visibleServiceIndexes := visibleServiceIndexes()
	if len(visibleServiceIndexes) < 2 {
		return
	}
	activeIndexInVisibleSlice := slices.Index(visibleServiceIndexes, activeIndex)
	if right {
		activeIndexInVisibleSlice++
		if activeIndexInVisibleSlice >= len(visibleServiceIndexes) {
			activeIndexInVisibleSlice = 0
		}
	} else {
		activeIndexInVisibleSlice--
		if activeIndexInVisibleSlice < 0 {
			activeIndexInVisibleSlice = len(visibleServiceIndexes) - 1
		}
	}
	activeIndex = visibleServiceIndexes[activeIndexInVisibleSlice]
	activeService = services[activeIndex]

	saveContextSettings()
}

func visibleServiceIndexes() []int {
	visibleServiceIndexes := make([]int, 0, len(services))

	for i := range services {
		services[i].StateMutex.RLock()
		if !onlyActive || services[i].State != service.StateStopped || i == activeIndex {
			visibleServiceIndexes = append(visibleServiceIndexes, i)
		}
		services[i].StateMutex.RUnlock()
	}

	return visibleServiceIndexes
}

func swap(increment int) {
	if onlyActive || len(services) < 2 {
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

func getKeyPressInfo(msg tea.KeyPressMsg) []string {
	return []string{
		"New key press:",
		fmt.Sprintf("msg.BaseCode: %c", msg.BaseCode),
		fmt.Sprintf("msg.Code: %c", msg.Code),
		fmt.Sprintf("msg.Mod: %d", msg.Mod),
		fmt.Sprintf("msg.ShiftedCode: %c", msg.ShiftedCode),
		fmt.Sprintf("msg.Text: %s", msg.Text),
		fmt.Sprintf("msg.String(): %s", msg.String()),
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	var activeLog *log.Log
	activeMutex.RLock()
	if showHelp {
		activeLog = help
	} else {
		activeLog = activeService.Log
	}
	activeMutex.RUnlock()

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.IsRepeat {
			break
		}
		k := msg.String()
		if debugKeyboard {
			for _, info := range getKeyPressInfo(msg) {
				activeLog.AddContent(info, true)
			}
		}
		var keyConsumed bool
		keyConsumed, cmd = activeLog.HandleKey(msg)
		if keyConsumed {
			break
		}

		if k == "ctrl+c" || k == "q" || k == "esc" {
			if showHelp && k != "ctrl+c" {
				showHelp = false
				break
			}
			showHelp = false
			if stopAllServices(true) {
				return m, tea.Quit
			}
			break
		} else if showHelp {
			if msg.Key().Code == '?' || k == "enter" || k == "space" {
				showHelp = false
			}
		} else {
			if k == "s" {
				stopAllServices(false)
			} else if k == "enter" || k == "space" {
				activeMutex.RLock()
				activeService.StateMutex.Lock()
				switch activeService.State {
				case service.StateStopped:
					if !quitting {
						activeService.StartService()
					}
				case service.StateStarting:
					activeService.EndService()
				case service.StateRunning:
					activeService.EndService()
				case service.StateStopping:
					activeService.EndService()
				}
				activeService.StateMutex.Unlock()
				activeMutex.RUnlock()
			} else if k == "left" || k == "h" {
				changeActive(false)
			} else if k == "right" || k == "l" {
				changeActive(true)
			} else if k == "shift+left" || k == "shift+h" {
				swap(-1)
			} else if k == "shift+right" || k == "shift+l" {
				swap(1)
			} else if k == "a" {
				onlyActive = !onlyActive
			} else if msg.Key().Code == '?' {
				showHelp = true
			}
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
		} else {
			for i := range services {
				if slices.Contains(services[i].WaitList, msg.Service) {
					startService(msg.Service)
					break
				}
			}
		}

	case service.ServiceStartedMsg:
		for i := range services {
			services[i].DoneWaiting(msg.Service)
		}

	case service.StartServiceMsg:
		startService(msg.Service)

	case lock.LockReleaseMsg:
		for i := range services {
			services[i].HandleUnlock(msg.Locks)
		}

	case tea.WindowSizeMsg:
		activeMutex.RLock()
		headerHeight := lipgloss.Height(m.headerView(msg.Width))
		footerHeight := lipgloss.Height(m.footerView(msg.Width))
		m.height = msg.Height
		m.width = msg.Width

		setLogSizes(msg.Width, msg.Height, headerHeight, footerHeight)

		if !m.ready {
			m.ready = true
		}

		activeMutex.RUnlock()
	default:
		cmd = activeLog.HandleNonKeyMsg(msg)
	}

	return m, cmd
}

func setLogSizes(width int, height int, headerHeight int, footerHeight int) {
	logHeight := height - headerHeight - footerHeight - 1
	help.SetSize(width, logHeight)
	for i := range services {
		services[i].Log.SetSize(width, logHeight)
	}
}

func stopAllServices(quit bool) bool {
	if quitting {
		return false
	}
	quitting = quit
	var anyRunning bool
	for i := range services {
		services[i].StateMutex.Lock()
		if services[i].State != service.StateStopped {
			anyRunning = true
			services[i].EndService()
		}
		services[i].StateMutex.Unlock()
	}
	return quitting && !anyRunning
}

func startService(serviceKey string) {
	if quitting {
		return
	}
	service := service.Services[serviceKey]
	service.StateMutex.Lock()
	service.StartService()
	service.StateMutex.Unlock()
}

func (m model) View() tea.View {
	v := tea.View{
		AltScreen: true,
	}

	if !m.ready {
		v.SetContent("Initializing...")
		return v
	}
	activeMutex.RLock()
	defer activeMutex.RUnlock()

	var activeLog *log.Log
	if showHelp {
		activeLog = help
	} else {
		activeLog = activeService.Log
	}

	headerView := m.headerView(m.width)
	footerView := m.footerView(m.width)

	headerHeight := lipgloss.Height(headerView)
	footerHeight := lipgloss.Height(footerView)

	if headerHeight+footerHeight+activeLog.GetHeight()+1 != m.height {
		setLogSizes(m.width, m.height, headerHeight, footerHeight)
	}

	logView, _ := activeLog.View()

	v.SetContent(fmt.Sprintf("%s\n%s\n%s\n%s", headerView, logView, footerView, activeLog.InputView()))
	return v
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

	if showHelp {
		title = activeCmdStyle.Render(" Help ")
	} else {
		for _, i := range visibleServiceIndexes() {
			var tabStyle *lipgloss.Style
			var stateStyle *lipgloss.Style
			services[i].StateMutex.RLock()
			switch services[i].State {
			case service.StateRunning:
				stateStyle = &runningStyle
			case service.StateStopping:
				stateStyle = &stoppingStyle
			case service.StateStarting:
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
	lipgloss.Color("#120ce3"),
	lipgloss.Color("#0000c3"),
	lipgloss.Color("#0000a3"),
}

func (m model) footerView(width int) string {

	var statusBars [][]string
	if debugScroll {
		var activeLog *log.Log
		if showHelp {
			activeLog = help
		} else {
			activeLog = activeService.Log
		}
		statusBars = append(statusBars, activeLog.ScrollDebug())
	}
	if !showHelp {
		statusBarItems := []string{
			context.Name,
			fmt.Sprintf("%d/%d running", runningServiceCount(), len(services)),
		}

		if activeService.Touched {
			statusBarItems = append(statusBarItems, fmt.Sprintf("Log: %s", formatDataSize(activeService.Log.GetContentSize())))
		}

		if activeService.Pid != 0 {
			statusBarItems = append(statusBarItems, fmt.Sprintf("PID %d", activeService.Pid))
		}

		var status string

		if activeService.State == service.StateStopping {
			status = "Stopping"
		} else if activeService.State == service.StateStarting {
			if len(activeService.WaitList) != 0 {
				status = "Waiting for: " + strings.Join(activeService.WaitList, ", ")
			} else {
				status = "Starting"
			}
		}
		if status != "" {
			statusBarItems = append(statusBarItems, status)
		}

		if quitting {
			statusBarItems = slices.Insert(statusBarItems, 1, "Quitting")
		}

		statusBars = append(statusBars, statusBarItems)
	}

	out := ""

	for i := range statusBars {
		if out != "" {
			out += "\n"
		}
		out += m.footerStatusBar(width, statusBars[i])
	}
	return out
}

func (m model) footerStatusBar(width int, statusBarItems []string) string {
	renderItems := make([]string, len(statusBarItems)*2)

	for i, item := range statusBarItems {
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

var context *Context
var services []*service.Service
var activeService *service.Service
var activeIndex = 0
var quitting bool
var configuration config.Config
var onlyActive bool
var showHelp bool
var filterLogs bool
var help *log.Log
var debugKeyboard bool
var debugScroll bool

var activeMutex sync.RWMutex

type contextDefinition struct {
	Name     string `yaml:"name"`
	Services map[string]struct {
		Name        string
		Path        string
		Commands    []service.Command
		Healthcheck service.Healthcheck
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
	service.Services = make(map[string]*service.Service)
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

		newService := service.New(serviceKey, name, servicePath, s.Commands, &configuration, s.Healthcheck)
		services[i] = &newService
		service.Services[serviceKey] = &newService
	}

	if context.Settings.ActiveService != "" {
		if savedActiveServiceIndex := slices.Index(finalServiceKeys, context.Settings.ActiveService); savedActiveServiceIndex != -1 {
			activeIndex = savedActiveServiceIndex
		}
	}
	activeService = services[activeIndex]
	help = log.New(&configuration)

	helpContent := []string{
		"",
		"Keys:",
		"",
		"[?] to toggle help screen",
		"[?] or [enter] or [space] or [q] or [esc] to exit help screen",
		"",
		"[ctrl+c] or [q] or [esc] to exit application",
		"",
		"[up] or [k] or [mouse_scrollup] to scroll log up",
		"[down] or [j] or [mouse_scrolldown] to scroll log down",
		"[page up] or [page down] to scroll up or down by screen height",
		"[b] or [t] to go to the bottom or top of the log",
		"",
		"[left] or [h] or [right] or [l] to move between services",
		"[shift+left] or [shift+h] or [shift+right] or [shift+l] to swap places between services",
		"",
		"[s] to stop all running services",
		"[a] to toggle between showing only running services",
		"",
		"[f] to enter filter mode",
		"[/] to enter search mode",
		"[q] or [esc] to exit filter/search mode",
		"[tab] or [shift+tab] to change filter/search type (case insensitive, case sensitive or regex)",
		"[n] or [shift+n] to move between search results",
	}

	for _, line := range helpContent {
		help.AddContent(line, true)
	}
	help.GotoTop()
	for i := range services {
		services[i].Log.AddContent("", true)
		services[i].Log.AddContent("Press [enter] or [space] to start.", true)
		services[i].Log.AddContent("Press [?] to see all key bindings.", true)
	}

	p = tea.NewProgram(
		model{},
	)
	for i := range services {
		services[i].Program = p
	}

	if len(os.Args) > 2 {
		flags := os.Args[2:]
		if slices.Contains(flags, "--debug-keyboard") {
			debugKeyboard = true
		}
		if slices.Contains(flags, "--debug-scroll") {
			debugScroll = true
		}
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
// TODO: handle too many elements on footer for viewport width
// TODO: possibility to add timestamps to system messages
// TODO: possibility to add timestamps to std messages
// TODO: remember timestamp rules per context service
// TODO: style sysout messages
// TODO: style syserr messages
// TODO: allow overriding success codes for commands
// TODO: automatically send second stop after 30s and then every 5s after that
// TODO: make windows gradle/maven/java kill optional
// TODO: add kill options as regex to config
// TODO: add command replacement regex to config
// TODO: show if tabs are filtered somewhere
// TODO: handle minimum window size
// TODO: readme
// TODO: context examples
