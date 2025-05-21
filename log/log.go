package log

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andresrobam/leggo/config"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Log struct {
	width                       int
	height                      int
	lines                       []string
	filteredLines               []int
	filter                      string
	filterMode                  InputMode
	filterErrorMessage          string
	search                      string
	searchMode                  InputMode
	searchResults               []SearchResult
	searchResultsByLine         map[int][]SearchResult
	searchResultIndex           int
	searchErrorMessage          string
	input                       textinput.Model
	lastLineOpen                bool
	currentLine                 int
	currentLineOffset           int
	currentLineOffsetPercentage float32
	configuration               *config.Config
	contentMutex                sync.RWMutex
	contentUpdated              atomic.Bool
	view                        string
	size                        int
	mode                        Mode
}

type Mode int

const (
	ModeNormal = iota
	ModeSearchInput
	ModeSearchNavigation
	ModeFilterInput
	ModeFiltered
)

type InputMode int

const (
	InputModeCaseInsensitive = iota
	InputModeCaseSensitive
	InputModeRegex
)

type SearchResult struct {
	line     int
	startCol int
	endCol   int
}

func (l *Log) find(search string, searchMode InputMode) {
	l.contentMutex.RLock()
	defer l.contentMutex.RUnlock()
	defer l.contentUpdated.Store(true)
	l.search = search
	l.searchResults = []SearchResult{}
	l.searchResultsByLine = map[int][]SearchResult{}
	l.searchResultIndex = 0
	l.searchMode = searchMode
	if l.search == "" || len(l.lines) == 0 {
		return
	}
	var regex string
	switch l.searchMode {
	case InputModeCaseInsensitive:
		regex = regexp.QuoteMeta(strings.ToLower(l.search))
	case InputModeCaseSensitive:
		regex = regexp.QuoteMeta(l.search)
	case InputModeRegex:
		regex = l.search
	}
	searchRegex, err := regexp.Compile(regex)
	if err != nil {
		l.searchErrorMessage = "Invalid regex"
		return
	}
	l.searchErrorMessage = ""
	for i := range l.lines {
		var line string
		if l.searchMode == InputModeCaseInsensitive {
			line = strings.ToLower(l.lines[i])
		} else {
			line = l.lines[i]
		}
		for _, searchResult := range searchRegex.FindAllStringIndex(line, -1) {
			resultStruct := SearchResult{
				line:     i,
				startCol: searchResult[0],
				endCol:   searchResult[1],
			}
			l.searchResults = append(l.searchResults, resultStruct)
			l.searchResultsByLine[i] = append(l.searchResultsByLine[i], resultStruct)
		}
	}
}

func (l *Log) filterResults(filter string, filterMode InputMode) {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	defer l.contentUpdated.Store(true)
	l.filter = filter
	l.filteredLines = make([]int, 0, 50)
	l.filterMode = filterMode
	l.currentLineOffset = 0
	l.currentLineOffsetPercentage = 0
	l.currentLine = max(len(l.lines)-1, 0)
	if l.filter == "" {
		return
	}
	l.currentLine = 0
	var regex string
	switch l.filterMode {
	case InputModeCaseInsensitive:
		regex = regexp.QuoteMeta(strings.ToLower(l.filter))
	case InputModeCaseSensitive:
		regex = regexp.QuoteMeta(l.filter)
	case InputModeRegex:
		regex = l.filter
	}
	filterRegex, err := regexp.Compile(regex)
	if err != nil {
		l.filterErrorMessage = "Invalid regex"
		return
	}
	l.searchErrorMessage = ""
	for i := range l.lines {
		var line string
		if l.filterMode == InputModeCaseInsensitive {
			line = strings.ToLower(l.lines[i])
		} else {
			line = l.lines[i]
		}
		if filterRegex.MatchString(line) {
			l.filteredLines = append(l.filteredLines, i)
		}
	}
	l.currentLine = max(len(l.filteredLines)-1, 0)
}

func (l *Log) GetHeight() int {
	return l.height
}

func (l *Log) HandleKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// TODO: show keybinds for moving between modes somewhere
	k := msg.String()
	if k == "ctrl+c" {
		return false, nil
	}
	switch l.mode {
	// TODO: fix scrolling in other modes
	case ModeNormal:
		if k == "up" || k == "k" {
			l.Scroll(-1)
			return true, nil
		} else if k == "down" || k == "j" {
			l.Scroll(1)
			return true, nil
		} else if k == "pgup" {
			l.Scroll(-l.GetHeight())
			return true, nil
		} else if k == "pgdown" {
			l.Scroll(l.GetHeight())
			return true, nil
		} else if k == "b" {
			l.GotoBottom()
			return true, nil
		} else if k == "t" {
			l.GotoTop()
			return true, nil
		} else if msg.Key().Code == '/' {
			l.setMode(ModeSearchInput)
			return true, nil
		} else if k == "f" {
			l.setMode(ModeFilterInput)
			return true, nil
		}
	case ModeSearchInput:
		if k == "esc" {
			l.setMode(ModeNormal)
			return true, nil
		} else if k == "enter" {
			l.setMode(ModeSearchNavigation)
			return true, nil
		} else if k == "tab" {
			l.find(l.search, getNextMode(l.searchMode)) // TODO: show keybind somewhere
			return true, nil
		} else if k == "shift+tab" {
			l.find(l.search, getPreviousMode(l.searchMode)) // TODO: show keybind somewhere
			return true, nil
		} else {
			var cmd tea.Cmd
			l.input, cmd = l.input.Update(msg)
			l.find(l.input.Value(), l.searchMode)
			return true, cmd
		}
	case ModeSearchNavigation:
		if k == "esc" || k == "q" {
			l.setMode(ModeNormal)
			return true, nil
		} else if msg.Key().Code == '/' {
			l.setMode(ModeSearchInput)
			return true, nil
		} else if k == "f" {
			l.setMode(ModeFilterInput)
			return true, nil
		} else if k == "n" { // TODO: show keybind somewhere
			l.shiftSearchResult(1)
			return true, nil
		} else if k == "N" { // TODO: show keybind somewhere
			l.shiftSearchResult(-1)
			return true, nil
		}
	case ModeFilterInput:
		if k == "esc" {
			l.setMode(ModeNormal)
			return true, nil
		} else if k == "enter" {
			l.setMode(ModeFiltered)
			return true, nil
		} else if k == "tab" {
			l.filterResults(l.filter, getNextMode(l.filterMode)) // TODO: show keybind somewhere
			return true, nil
		} else if k == "shift+tab" {
			l.filterResults(l.filter, getNextMode(l.filterMode)) // TODO: show keybind somewhere
			return true, nil
		} else {
			var cmd tea.Cmd
			l.input, cmd = l.input.Update(msg)
			l.filterResults(l.input.Value(), l.filterMode)
			return true, cmd
		}
	case ModeFiltered:
		if k == "esc" || k == "q" {
			l.setMode(ModeNormal)
			return true, nil
		} else if msg.Key().Code == '/' {
			l.setMode(ModeSearchInput)
			return true, nil
		} else if k == "f" {
			l.setMode(ModeFilterInput)
			return true, nil
		}
	}
	return false, nil
}

func getNextMode(mode InputMode) InputMode {
	switch mode {
	case InputModeCaseInsensitive:
		return InputModeCaseSensitive
	case InputModeCaseSensitive:
		return InputModeRegex
	default:
		return InputModeCaseInsensitive
	}
}

func getPreviousMode(mode InputMode) InputMode {
	switch mode {
	case InputModeRegex:
		return InputModeCaseSensitive
	case InputModeCaseSensitive:
		return InputModeCaseInsensitive
	default:
		return InputModeRegex
	}
}

func getLineOfCol(col int, lines []string) int {
	return 0 // TODO: dont return 0
}

func (l *Log) shiftSearchResult(offset int) {
	l.contentMutex.Lock()
	if len(l.searchResults) == 0 {
		return
	}
	l.searchResultIndex += offset
	if l.searchResultIndex < 0 {
		l.searchResultIndex = len(l.searchResults) - 1
	} else if l.searchResultIndex >= len(l.searchResults) {
		l.searchResultIndex = 0
	}

	searchResult := l.searchResults[l.searchResultIndex]
	line := l.activeLineWrapped(searchResult.line, false)
	l.currentLine = searchResult.line
	l.currentLineOffset = -(getLineOfCol(searchResult.startCol, line) + getLineOfCol(searchResult.endCol-1, line)) / 2
	l.contentMutex.Unlock()
	l.Scroll(l.height / 2)
}

func (l *Log) setMode(mode Mode) {
	l.contentMutex.Lock()
	previousMode := l.mode
	l.mode = mode

	switch mode {
	case ModeFilterInput, ModeSearchInput:
		l.input.Focus()
	default:
		l.input.Blur()
	}

	switch mode {
	case ModeFilterInput:
		defer l.filterResults(l.filter, l.filterMode)
		l.input.SetValue(l.filter)
		l.input.CursorEnd()
	case ModeFiltered:
	default:
		l.filteredLines = make([]int, 0, 50)
	}

	switch mode {
	case ModeSearchInput:
		defer l.find(l.search, l.searchMode)
		l.input.SetValue(l.search)
		l.input.CursorEnd()
	case ModeSearchNavigation:
	default:
		l.searchResultIndex = 0
		l.searchResults = []SearchResult{}
		l.searchResultsByLine = map[int][]SearchResult{}
	}

	if (mode != ModeFilterInput && mode != ModeFiltered) && (previousMode == ModeFilterInput || previousMode == ModeFiltered) {
		l.currentLine = l.activeLineCount() - 1
		l.currentLineOffset = 0
		l.currentLineOffsetPercentage = 0
	}

	l.contentUpdated.Store(true)
	l.contentMutex.Unlock()
}

func (l *Log) HandleNonKeyMsg(msg tea.Msg) (cmd tea.Cmd) {
	if l.mode != ModeFilterInput && l.mode != ModeSearchInput {
		return nil
	}
	l.input, cmd = l.input.Update(msg)
	return
}

func New(configuration *config.Config) *Log {
	log := &Log{
		configuration: configuration,
		lines:         make([]string, 0, 50),
		filteredLines: make([]int, 0, 50),
		input:         textinput.New(),
	}
	log.contentUpdated.Store(true)
	log.input.CharLimit = 100
	return log
}

func (l *Log) GetContentSize() int {
	return l.size
}

func (l *Log) filterActive() bool {
	return l.filter != "" && (l.mode == ModeFilterInput || l.mode == ModeFiltered)
}

func (l *Log) activeLineCount() int {
	if l.filterActive() {
		return len(l.filteredLines)
	} else {
		return len(l.lines)
	}
}

var searchResultStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#000000")).
	Background(lipgloss.Color("#fade07"))
var currentSearchResultStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#000000")).
	Background(lipgloss.Color("#a69514"))

func (l *Log) activeLineWrapped(index int, colorSearchResults bool) []string {
	var line string
	if l.filterActive() {
		line = l.lines[l.filteredLines[index]]
	} else {
		line = l.lines[index]
	}
	if colorSearchResults && line != "" {
		if searchResults, ok := l.searchResultsByLine[index]; ok {
			var coloredLine string
			for i, searchResult := range searchResults {
				var prefix string
				if i == 0 && searchResult.startCol > 0 {
					prefix = line[:searchResult.startCol]
				} else if i > 0 && searchResult.startCol > searchResults[i-1].endCol {
					prefix = line[searchResults[i-1].endCol:searchResult.startCol]
				}
				var style lipgloss.Style
				if searchResult == l.searchResults[l.searchResultIndex] {
					style = currentSearchResultStyle
				} else {
					style = searchResultStyle
				}
				coloredLine += prefix + style.Render(line[searchResult.startCol:searchResult.endCol])
			}
			line = coloredLine + line[searchResults[len(searchResults)-1].endCol:]
		}
	}
	return strings.Split(ansi.Hardwrap(line, l.width, true), "\n")
}

func (l *Log) View() (string, bool) {
	if !l.contentUpdated.Swap(false) {
		return l.view, false
	}
	l.contentMutex.RLock()
	defer l.contentMutex.RUnlock()
	content := lipgloss.NewStyle().
		Height(l.height).
		MaxWidth(l.width).
		MaxHeight(l.height).
		Render(strings.Join(l.getVisibleLines(), "\n"))
	l.view = content
	return content, true
}

func (l *Log) clampCurrentLine() {
	if l.activeLineCount() == 0 {
		l.currentLine = 0
		l.currentLineOffset = 0
		l.currentLineOffsetPercentage = 0
		return
	}
	if l.currentLine >= l.height-1 {
		return
	}
	scrollAmount := l.height - len(l.getVisibleLines())
	for range scrollAmount {
		if l.scroll(false) {
			return
		}
	}
}

func (l *Log) Scroll(amount int) {
	if amount == 0 {
		return
	}
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()

	if l.activeLineCount() == 0 {
		return
	}

	up := amount < 0
	if up {
		amount *= -1
	}
	for range amount {
		if l.scroll(up) {
			break
		}
	}
	l.clampCurrentLine()
	l.contentUpdated.Store(true)
}

func (l *Log) ScrollDebug() []string {
	l.contentMutex.RLock()
	defer l.contentMutex.RUnlock()
	return []string{
		fmt.Sprintf("currentline: %d", l.currentLine),
		fmt.Sprintf("currentLineOffset: %d", l.currentLineOffset),
		fmt.Sprintf("currentLineHeight: %d", l.getLineHeight(l.currentLine)),
		fmt.Sprintf("currentLineOffsetPercentage: %.2f", l.currentLineOffsetPercentage),
	}
}

func (l *Log) ModeDebug() []string {
	var mode string
	switch l.mode {
	case ModeFilterInput:
		mode = "filterInput"
	case ModeFiltered:
		mode = "filtered"
	case ModeSearchInput:
		mode = "searchInput"
	case ModeSearchNavigation:
		mode = "searchNav"
	default:
		mode = "normal"
	}
	return []string{
		fmt.Sprintf("mode: %s", mode),
		fmt.Sprintf("search: %s", l.search),
		fmt.Sprintf("filter: %s", l.filter),
		fmt.Sprintf("inputValue: %s", l.input.Value()),
		fmt.Sprintf("inputFocus: %t", l.input.Focused()),
		fmt.Sprintf("filteredLines: %d", len(l.filteredLines)),
		fmt.Sprintf("searchResults: %d", len(l.searchResults)),
	}
}

func (l *Log) getLineHeight(i int) int {
	if l.activeLineCount() == 0 {
		return 0
	}
	return len(l.activeLineWrapped(i, false))
}

func (l *Log) recalculateCurrentLineOffsetPercentage() {
	l.recalculateCurrentLineOffsetPercentageWithHeight(l.getLineHeight(l.currentLine))
}
func (l *Log) recalculateCurrentLineOffsetPercentageWithHeight(lineHeight int) {
	if lineHeight < 2 {
		l.currentLineOffsetPercentage = 0
		return
	}
	l.currentLineOffsetPercentage = float32(l.currentLineOffset) / (float32(lineHeight) - 1)
}

func (l *Log) matchesFilter(line string) bool {
	if !l.filterActive() {
		return false
	}
	switch l.filterMode {
	case InputModeCaseInsensitive:
		return strings.Contains(strings.ToLower(line), strings.ToLower(l.filter))
	case InputModeCaseSensitive:
		return strings.Contains(line, l.filter)
	case InputModeRegex:
		filterRegex, err := regexp.Compile(l.filter)
		if err != nil {
			return false
		}
		return filterRegex.MatchString(line)
	}
	return false
}

func (l *Log) scroll(up bool) bool {
	if up { // scrolling up
		currentLineHeight := l.getLineHeight(l.currentLine)
		if -l.currentLineOffset+1 >= currentLineHeight { // at top of current line
			if l.currentLine == 0 { // at top of log
				return true
			}
			l.currentLine--
			l.currentLineOffset = 0
			l.currentLineOffsetPercentage = 0
		} else { // in the middle of current line
			l.currentLineOffset--
			l.recalculateCurrentLineOffsetPercentageWithHeight(currentLineHeight)
		}

	} else { // scrolling down
		if l.currentLineOffset >= 0 { // at bottom of current line
			if l.currentLine == l.activeLineCount()-1 { // at bottom of log
				return true
			}
			l.currentLine++
			currentLineHeight := l.getLineHeight(l.currentLine)
			l.currentLineOffset = -currentLineHeight + 1
			l.recalculateCurrentLineOffsetPercentageWithHeight(currentLineHeight)
		} else { // in the middle of current line
			l.currentLineOffset++
			l.recalculateCurrentLineOffsetPercentage()
		}
	}
	return false
}

func (l *Log) GotoTop() {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	if l.activeLineCount() == 0 {
		return
	}
	l.currentLine = 0
	lineHeight := l.getLineHeight(l.currentLine)
	l.currentLineOffset = -lineHeight + 1
	l.recalculateCurrentLineOffsetPercentageWithHeight(lineHeight)
	l.clampCurrentLine()
	l.contentUpdated.Store(true)
}

func (l *Log) GotoBottom() {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	if l.activeLineCount() == 0 {
		return
	}
	l.currentLine = l.activeLineCount() - 1
	l.currentLineOffset = 0
	l.currentLineOffsetPercentage = 0
	l.contentUpdated.Store(true)
}

func (l *Log) AtBottom() bool {
	return l.activeLineCount() == 0 || (l.currentLine == l.activeLineCount()-1 && l.currentLineOffset == 0)
}

func (l *Log) SetSize(width int, height int) {
	l.contentMutex.Lock()
	defer l.clampCurrentLine()
	defer l.contentMutex.Unlock()
	defer l.contentUpdated.Store(true)
	l.input.SetWidth(10)
	l.width = width
	l.height = height
	if l.activeLineCount() == 0 {
		return
	}
	currentLineHeight := l.getLineHeight(l.currentLine)
	if currentLineHeight == 0 {
		return
	}
	l.currentLineOffset = int(l.currentLineOffsetPercentage * float32(currentLineHeight-1))
	l.recalculateCurrentLineOffsetPercentageWithHeight(currentLineHeight)
}

func (l *Log) getVisibleLines() []string {
	visibleLines := make([]string, l.height, l.height)

	if l.activeLineCount() == 0 {
		return visibleLines
	}
	screenLine := l.height - 1
outer:
	for i := l.currentLine; i >= 0; i-- {
		wrappedLines := l.activeLineWrapped(i, true)
		startingLine := len(wrappedLines) - 1
		if i == l.currentLine {
			startingLine += l.currentLineOffset
		}
		for j := startingLine; j >= 0; j-- {
			visibleLines[screenLine] = wrappedLines[j]
			screenLine--
			if screenLine < 0 {
				break outer
			}
		}
	}

	if screenLine >= 0 {
		visibleLines = visibleLines[screenLine+1:]
	}

	return visibleLines
}

func (l *Log) InputView() string {

	l.contentMutex.RLock()
	defer l.contentMutex.RUnlock()

	if l.mode == ModeNormal {
		return ""
	}

	var mode string
	switch l.mode {
	case ModeFilterInput, ModeFiltered:
		mode = "Filter"
	case ModeSearchInput, ModeSearchNavigation:
		mode = "Search"
	}

	return mode + ": " + l.input.View() + " " + l.inputViewRightSide()
}

func (l *Log) inputViewRightSide() string {

	var errorMessage string

	switch l.mode {
	case ModeFilterInput, ModeFiltered:
		errorMessage = l.filterErrorMessage
	case ModeSearchInput, ModeSearchNavigation:
		errorMessage = l.searchErrorMessage
	}

	if errorMessage != "" {
		return errorMessage
	}

	var inputMode InputMode
	var inputModeName string

	var results string

	switch l.mode {
	case ModeFilterInput, ModeFiltered:
		inputMode = l.filterMode
		if l.filter != "" {
			if len(l.filteredLines) == 0 {
				results = "No results"
			} else {
				results = fmt.Sprintf("%d", len(l.filteredLines))
			}
		}
	case ModeSearchInput, ModeSearchNavigation:
		inputMode = l.searchMode
		if l.search != "" {
			if len(l.searchResults) == 0 {
				results = "No results"
			} else {
				results = fmt.Sprintf("%d/%d", l.searchResultIndex+1, len(l.searchResults))
			}
		}
	}

	switch inputMode {
	case InputModeCaseInsensitive:
		inputModeName = "case insensitive"
	case InputModeCaseSensitive:
		inputModeName = "case sensitive"
	case InputModeRegex:
		inputModeName = "regex"
	}

	return inputModeName + " " + results
}

func (l *Log) clearOldLines() {

	l.size = 0
	for i := range l.lines {
		l.size += len(l.lines[i])
	}
	exceededBytes := l.size - l.configuration.MaxLogBytes
	if exceededBytes <= 0 {
		return
	}
	linesToDelete := 0
	for i := range l.lines {
		line := l.lines[i]
		l.size -= len(line)
		exceededBytes -= len(line)
		linesToDelete++
		if exceededBytes <= 0 {
			break
		}
	}
	for i := range l.filteredLines {
		l.filteredLines[i] -= linesToDelete
	}
	l.lines = l.lines[linesToDelete:]
	l.currentLine -= linesToDelete // TODO: reduce by number of filtered lines deleted if filtered, number = first n negative elements of filteredlines, remove those elements from filteredlines
	// TODO: remove from search results if in there
	l.clampCurrentLine()
}

func (l *Log) AddContent(addition string, endLine bool) {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	if l.lastLineOpen {
		lastLineIndex := len(l.lines) - 1
		l.lines[lastLineIndex] += addition
		if endLine {
			l.lastLineOpen = false
		}
		if l.matchesFilter(l.lines[lastLineIndex]) && (len(l.filteredLines) == 0 || l.filteredLines[len(l.filteredLines)-1] != lastLineIndex) {
			l.filteredLines = append(l.filteredLines, len(l.lines)-1)
		}
	} else {
		atLastLine := l.AtBottom()
		l.lines = append(l.lines, addition)
		if !endLine {
			l.lastLineOpen = true
		}
		if l.matchesFilter(addition) {
			l.filteredLines = append(l.filteredLines, len(l.lines)-1)
			if atLastLine && len(l.lines) != 1 {
				l.currentLine++
			}
		} else if l.mode == ModeNormal && atLastLine && len(l.lines) != 1 {
			l.currentLine++
		}
	}
	// TODO: add to search results if match
	l.clearOldLines()
	l.contentUpdated.Store(true)
}

func (l *Log) Clear() {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	l.lastLineOpen = false
	l.lines = make([]string, 0, 50)
	l.filteredLines = make([]int, 0, 50)
	l.searchResults = []SearchResult{}
	l.searchResultsByLine = map[int][]SearchResult{}
	l.searchResultIndex = 0
	l.currentLine = 0
	l.currentLineOffset = 0
	l.currentLineOffsetPercentage = 0
	l.contentUpdated.Store(true)
}
