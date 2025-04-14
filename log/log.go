package log

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andresrobam/leggo/config"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Log struct {
	width          int
	height         int
	lines          []string
	lastLineOpen   bool
	currentLine    int
	configuration  *config.Config
	contentMutex   sync.RWMutex
	contentUpdated atomic.Bool
	view           string
	size           int
}

func New(configuration *config.Config) *Log {
	log := &Log{
		configuration: configuration,
		lines:         make([]string, 0, 50),
	}
	log.contentUpdated.Store(true)
	return log
}

func (l *Log) GetContentSize() int {
	return l.size
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
	if l.currentLine < l.height-1 {
		if l.height >= len(l.lines) {
			l.currentLine = len(l.lines) - 1
		} else {
			l.currentLine = l.height - 1
		}
	} else if l.currentLine >= len(l.lines)-1 {
		l.currentLine = len(l.lines) - 1
	}
}

func (l *Log) Scroll(amount int) {
	l.currentLine += amount
	l.clampCurrentLine()
	l.contentUpdated.Store(true)
}

func (l *Log) GotoBottom() {
	if len(l.lines) == 0 {
		return
	}
	l.currentLine = len(l.lines) - 1
	l.contentUpdated.Store(true)
}

func (l *Log) SetSize(width int, height int) {
	l.width = width
	l.height = height
	l.contentUpdated.Store(true)
}

func (l *Log) getVisibleLines() []string {
	visibleLines := make([]string, l.height, l.height)

	if len(l.lines) == 0 {
		return visibleLines
	}

	screenLine := l.height - 1
outer:
	for i := l.currentLine; i >= 0; i-- {
		wrappedLines := strings.Split(ansi.Hardwrap(l.lines[i], l.width, true), "\n")
		for j := len(wrappedLines) - 1; j >= 0; j-- {
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
		l.size -= len(l.lines[i])
		exceededBytes -= len(l.lines[i])
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
	atLastLine := l.currentLine == (len(l.lines) - 1)
	if l.lastLineOpen {
		l.lines[len(l.lines)-1] += addition
		if endLine {
			l.lastLineOpen = false
		}
	} else {
		l.lines = append(l.lines, addition)
		if !endLine {
			l.lastLineOpen = true
		}
	}
	if endLine && atLastLine {
		l.currentLine++
	}
	l.clearOldLines()
	l.contentUpdated.Store(true)
}
