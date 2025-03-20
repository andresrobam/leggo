package log

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// New returns a new model with the given width and height
func New() (m Log) {
	m.setInitialValues()
	return m
}

// Model is the Bubble Tea model for this viewport element.
type Log struct {
	width  int
	height int

	// Whether or not to respond to the mouse. The mouse must be enabled in
	// Bubble Tea for this to work. For details, see the Bubble Tea docs.
	MouseWheelEnabled bool

	// The number of lines the mouse wheel will scroll. By default, this is 3.
	MouseWheelDelta int

	// YOffset is the vertical scroll position.
	YOffset int

	initialized bool
	lines       []string
}

func (m *Log) setInitialValues() {
	m.MouseWheelEnabled = true
	m.MouseWheelDelta = 3
	m.initialized = true
}

// Init exists to satisfy the tea.Model interface for composability purposes.
func (m Log) Init() (Log, tea.Cmd) {
	return m, nil
}

// Height returns the height of the viewport.
func (m Log) Height() int {
	return m.height
}

// SetHeight sets the height of the viewport.
func (m *Log) SetHeight(h int) {
	m.height = h
}

// Width returns the width of the viewport.
func (m Log) Width() int {
	return m.width
}

// SetWidth sets the width of the viewport.
func (m *Log) SetWidth(w int) {
	m.width = w
}

// AtTop returns whether or not the viewport is at the very top position.
func (m Log) AtTop() bool {
	return m.YOffset <= 0
}

// AtBottom returns whether or not the viewport is at or past the very bottom
// position.
func (m Log) AtBottom() bool {
	return m.YOffset >= m.maxYOffset()
}

// PastBottom returns whether or not the viewport is scrolled beyond the last
// line. This can happen when adjusting the viewport height.
func (m Log) PastBottom() bool {
	return m.YOffset > m.maxYOffset()
}

// ScrollPercentInt returns the amount scrolled as an int between 0 and 100.
func (m Log) ScrollPercent() int {
	if m.Height() >= len(m.lines) {
		return 100
	}
	v := 100 * m.YOffset / (len(m.lines) - m.Height())
	if v <= 0 {
		return 0
	} else if v >= 100 {
		return 100
	} else {
		return v
	}
}

// SetContent set the pager's text content.
func (m *Log) SetContent(s string) {
	m.lines = strings.Split(s, "\n")

	if m.YOffset > len(m.lines)-1 {
		m.GotoBottom()
	}
}

// maxYOffset returns the maximum possible value of the y-offset based on the
// viewport's content and set height.
func (m Log) maxYOffset() int {
	return max(0, len(m.lines)-m.Height())
}

// visibleLines returns the lines that should currently be visible in the
// viewport.
func (m Log) visibleLines() (lines []string) {
	if len(m.lines) > 0 {
		top := max(0, m.YOffset)
		bottom := clamp(m.YOffset+m.Height(), top, len(m.lines))
		lines = m.lines[top:bottom]
	}
	return lines
}

// SetYOffset sets the Y offset.
func (m *Log) SetYOffset(n int) {
	m.YOffset = clamp(n, 0, m.maxYOffset())
}

// ViewDown moves the view down by the number of lines in the viewport.
// Basically, "page down".
func (m *Log) ViewDown() {
	if m.AtBottom() {
		return
	}

	m.LineDown(m.Height())
}

// ViewUp moves the view up by one height of the viewport. Basically, "page up".
func (m *Log) ViewUp() {
	if m.AtTop() {
		return
	}

	m.LineUp(m.Height())
}

// HalfViewDown moves the view down by half the height of the viewport.
func (m *Log) HalfViewDown() {
	if m.AtBottom() {
		return
	}

	m.LineDown(m.Height() / 2) //nolint:mnd
}

// HalfViewUp moves the view up by half the height of the viewport.
func (m *Log) HalfViewUp() {
	if m.AtTop() {
		return
	}

	m.LineUp(m.Height() / 2) //nolint:mnd
}

// LineDown moves the view down by the given number of lines.
func (m *Log) LineDown(n int) {
	if m.AtBottom() || n == 0 || len(m.lines) == 0 {
		return
	}

	// Make sure the number of lines by which we're going to scroll isn't
	// greater than the number of lines we actually have left before we reach
	// the bottom.
	m.SetYOffset(m.YOffset + n)
}

// LineUp moves the view down by the given number of lines. Returns the new
// lines to show.
func (m *Log) LineUp(n int) {
	if m.AtTop() || n == 0 || len(m.lines) == 0 {
		return
	}

	// Make sure the number of lines by which we're going to scroll isn't
	// greater than the number of lines we are from the top.
	m.SetYOffset(m.YOffset - n)
}

// TotalLineCount returns the total number of lines (both hidden and visible) within the viewport.
func (m Log) TotalLineCount() int {
	return len(m.lines)
}

// VisibleLineCount returns the number of the visible lines within the viewport.
func (m Log) VisibleLineCount() int {
	return len(m.visibleLines())
}

// GotoTop sets the viewport to the top position.
func (m *Log) GotoTop() (lines []string) {
	if m.AtTop() {
		return nil
	}

	m.SetYOffset(0)
	return m.visibleLines()
}

// GotoBottom sets the viewport to the bottom position.
func (m *Log) GotoBottom() (lines []string) {
	m.SetYOffset(m.maxYOffset())
	return m.visibleLines()
}

// View renders the viewport into a string.
func (m Log) View() string {
	w, h := m.Width(), m.Height()

	return lipgloss.NewStyle().
		Width(m.width). // pad to width.
		Height(h).      // pad to height.
		MaxHeight(h).   // truncate height if taller.
		MaxWidth(w).    // truncate width if wider.
		Render(strings.Join(m.visibleLines(), "\n"))
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
