package log

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andresrobam/leggo/config"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Log struct {
	width                       int
	height                      int
	lines                       []string
	filteredLines               []*string
	filter                      string
	filterMode                  InputMode
	search                      string
	searchMode                  InputMode
	searchResults               []SearchResult
	searchResultIndex           int
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

func (l *Log) GetHeight() int {
	return l.height
}

func (l *Log) HandleKey(msg tea.KeyPressMsg) bool {
	k := msg.String()
	if k == "ctrl+c" {
		return false
	}
	switch l.mode {
	case ModeNormal:
		if k == "up" || k == "k" {
			l.Scroll(-1)
			return true
		} else if k == "down" || k == "j" {
			l.Scroll(1)
			return true
		} else if k == "pgup" {
			l.Scroll(-l.GetHeight())
			return true
		} else if k == "pgdown" {
			l.Scroll(l.GetHeight())
			return true
		} else if k == "b" {
			l.GotoBottom()
			return true
		} else if k == "t" {
			l.GotoTop()
			return true
		} else if msg.Key().Code == '/' {
			l.mode = ModeSearchInput
			return true
		} else if k == "f" {
			l.mode = ModeFilterInput
			return true
		}
	case ModeSearchInput:
		if k == "esc" {
			l.mode = ModeNormal
			return true
		} else if k == "enter" {
			l.mode = ModeSearchNavigation
			return true
		} else {
			// TODO: pass to input
			// TODO: handle search string change
			return true
		}

	// TODO: tab swap mode forwards
	// TODO: shift+tab swap mode backwards
	case ModeSearchNavigation:
		// TODO: n next
		// TODO: shift+n previous
		if k == "esc" || k == "q" {
			l.mode = ModeNormal
			return true
		} else if msg.Key().Code == '/' {
			l.mode = ModeSearchInput
			return true
		} else if k == "f" {
			l.mode = ModeFilterInput
			return true
		}
	case ModeFilterInput:
		if k == "esc" {
			l.mode = ModeNormal
			return true
		} else if k == "enter" {
			l.mode = ModeFiltered
			return true
		} else {
			// TODO: pass to input
			return true
		}
	// TODO: tab swap mode forwards
	// TODO: shift+tab swap mode backwards
	case ModeFiltered:
		if k == "esc" || k == "q" {
			l.mode = ModeNormal
			return true
		} else if msg.Key().Code == '/' {
			l.mode = ModeSearchInput
			return true
		} else if k == "f" {
			l.mode = ModeFilterInput
			return true
		}
	}
	return false
}

func New(configuration *config.Config) *Log {
	log := &Log{
		configuration: configuration,
		lines:         make([]string, 0, 50),
		filteredLines: make([]*string, 0, 50),
	}
	log.contentUpdated.Store(true)
	return log
}

func (l *Log) GetContentSize() int {
	return l.size
}

func (l *Log) ActiveLineCount() int {
	switch l.mode {
	case ModeFilterInput:
		fallthrough
	case ModeFiltered:
		return len(l.filteredLines)
	default:
		return len(l.lines)
	}
}

func (l *Log) activeLineWrapped(index int) []string {
	var line string
	switch l.mode {
	case ModeFilterInput:
		fallthrough
	case ModeFiltered:
		line = *l.filteredLines[index]
	default:
		line = l.lines[index]
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
	if l.ActiveLineCount() == 0 {
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

	if l.ActiveLineCount() == 0 {
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
	return []string{
		fmt.Sprintf("currentline: %d", l.currentLine),
		fmt.Sprintf("currentLineOffset: %d", l.currentLineOffset),
		fmt.Sprintf("currentLineHeight: %d", l.getLineHeight(l.currentLine)),
		fmt.Sprintf("currentLineOffsetPercentage: %.2f", l.currentLineOffsetPercentage),
	}
}

func (l *Log) getLineHeight(i int) int {
	if l.ActiveLineCount() == 0 {
		return 0
	}
	return len(l.activeLineWrapped(i))
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
	if l.filter == "" {
		return false
	}
	return false
	// TODO:; check for match
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
			if l.currentLine == l.ActiveLineCount()-1 { // at bottom of log
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
	if l.ActiveLineCount() == 0 {
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
	if l.ActiveLineCount() == 0 {
		return
	}
	l.currentLine = l.ActiveLineCount() - 1
	l.currentLineOffset = 0
	l.currentLineOffsetPercentage = 0
	l.contentUpdated.Store(true)
}

func (l *Log) AtBottom() bool {
	return l.ActiveLineCount() == 0 || (l.currentLine == l.ActiveLineCount()-1 && l.currentLineOffset == 0)
}

func (l *Log) SetSize(width int, height int) {
	l.contentMutex.Lock()
	defer l.clampCurrentLine()
	defer l.contentUpdated.Store(true)
	defer l.contentMutex.Unlock()
	l.width = width
	l.height = height
	if l.ActiveLineCount() == 0 {
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

	if l.ActiveLineCount() == 0 {
		return visibleLines
	}

	screenLine := l.height - 1
outer:
	for i := l.currentLine; i >= 0; i-- {
		wrappedLines := l.activeLineWrapped(i)
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
		filteredIndex := slices.Index(l.filteredLines, &line)
		if filteredIndex != -1 {
			l.filteredLines = slices.Delete(l.filteredLines, filteredIndex, filteredIndex+1)
		}
		linesToDelete++
		if exceededBytes <= 0 {
			break
		}
	}
	l.lines = l.lines[linesToDelete:]
	l.currentLine -= linesToDelete
	l.clampCurrentLine()
}

func (l *Log) AddContent(addition string, endLine bool) {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	if l.lastLineOpen {
		l.lines[len(l.lines)-1] += addition
		if endLine {
			l.lastLineOpen = false
		}
	} else {
		atLastLine := l.AtBottom()
		l.lines = append(l.lines, addition)
		if !endLine {
			l.lastLineOpen = true
		}
		if atLastLine && len(l.lines) != 1 {
			l.currentLine++
		}
	}
	lastLine := l.lines[len(l.lines)-1]
	if !slices.Contains(l.filteredLines, &lastLine) && l.matchesFilter(lastLine) {
		l.filteredLines = append(l.filteredLines, &lastLine)
	}
	l.clearOldLines()
	l.contentUpdated.Store(true)
}

func (l *Log) Clear() {
	l.contentMutex.Lock()
	defer l.contentMutex.Unlock()
	l.lastLineOpen = false
	l.lines = make([]string, 0, 50)
	l.filteredLines = make([]*string, 0, 50)
	l.currentLine = 0
	l.currentLineOffset = 0
	l.currentLineOffsetPercentage = 0
	l.contentUpdated.Store(true)
}
