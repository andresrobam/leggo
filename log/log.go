package log

import (
	"math"
	"strings"

	"github.com/andresrobam/leggo/config"
	"github.com/charmbracelet/x/ansi"
)

type Log struct {
	width                       int
	height                      int
	lines                       []string
	lastLineOpen                bool
	currentLine                 int
	currentLineOffset           int
	currentLineOffsetPercentage float64
	configuration               *config.Config
}

func New(configuration *config.Config) *Log {
	return &Log{
		configuration: configuration,
		lines:         make([]string, 0, configuration.InitialLineCapacity),
	}
}

func (l Log) getCurrentLineTotal() int {
	return strings.Count(ansi.Hardwrap(l.lines[l.currentLine], l.width, true), "\n") + 1
}

func (l Log) GetContent() string {
	return strings.Join(*l.getVisibleLines(), "\n")
}

func (l *Log) Scroll(amount int) {
	// currentLineTotal :=
	l.currentLine += amount
	if l.currentLine < 0 {
		l.currentLine = 0
	} else if l.currentLine >= len(l.lines)-1 {
		l.currentLine = len(l.lines) - 1
	}
	l.currentLineOffsetPercentage = float64(l.currentLineOffset) / float64(l.getCurrentLineTotal())
}

func (l *Log) recalculateCurrentLineOffset() {
	if l.currentLineOffset == 0 {
		return
	}
	currentLineTotal := l.getCurrentLineTotal()
	l.currentLineOffset = int(math.Round(l.currentLineOffsetPercentage * float64(currentLineTotal)))
	if l.currentLineOffset >= currentLineTotal {
		l.currentLineOffset = 0
	}
}

func (l *Log) setSize(width int, height int) {
	l.width = width
	l.height = height
	l.recalculateCurrentLineOffset()
}

func (l Log) getVisibleLines() *[]string {
	visibleLines := make([]string, l.height, l.height)

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

	return &visibleLines
}

func (l *Log) AddContent(addition *string, endLine bool) {
	if l.lastLineOpen {
		l.lines[len(l.lines)-1] += *addition
		if endLine {
			l.lastLineOpen = false
		}
	} else {
		l.lines = append(l.lines, *addition)
		if len(l.lines) == cap(l.lines) {
			newSlice := make([]string, int(float32(len(l.lines))*l.configuration.LineCapacityMultiplier))
			copy(newSlice, l.lines)
			l.lines = newSlice
		}
		if !endLine {
			l.lastLineOpen = true
		}
	}
}
