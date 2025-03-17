package service

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
)

type State int

const (
	StateStopped State = iota
	StateRunning
	StateStopping
)

type LineType int

const (
	LineTypeStdout LineType = iota
	LineTypeStderr
	LineTypeSysout
	LineTypeSyserr
)

type Line struct {
	Text string
	Time time.Time
	Type LineType
}

type Service struct {
	Name              string
	Path              string
	Commands          []string
	Lines             []Line
	ContentBytes      int
	currentStdoutLine *Line
	currentStderrLine *Line
	Active            bool
	State             State
	cmd               *exec.Cmd
	outPipe           *io.ReadCloser
	errPipe           *io.ReadCloser
	Program           *tea.Program
	StateMutex        sync.RWMutex
	LineMutex         sync.RWMutex
}

type ContentUpdateMsg struct{}

type ServiceStoppedMsg struct{}

type ServiceStoppingMsg struct{}

type ServiceStartedMsg struct{}

const maxBytes int = 10*1024
const splitLines bool = false

func (s *Service) clearOldLines() {
	exceededBytes := s.ContentBytes - maxBytes
	if exceededBytes <= 0 {
		return
	}
	var elementsToDelete int
	var bytesDeleted int
	for i := range s.Lines {
		lineBytes := len(s.Lines[i].Text)
		if !splitLines || lineBytes <= exceededBytes {
			elementsToDelete++
			exceededBytes -= lineBytes
			bytesDeleted += lineBytes
		} else {
			s.Lines[i].Text = s.Lines[i].Text[exceededBytes:]
			bytesDeleted += exceededBytes
			break
		}
		if exceededBytes <= 0 {
			break
		}
	}
	if elementsToDelete != 0 {
		s.Lines = s.Lines[elementsToDelete:]
	}
	s.ContentBytes -= bytesDeleted
}

func (s *Service) addOutput(addition *string, endLine bool, currentLine *Line, lineType LineType) (setNewLine bool, newLine *Line) {
	s.LineMutex.Lock()
	defer s.LineMutex.Unlock()
	if currentLine != nil {
		currentLine.Text += *addition
		if endLine {
			setNewLine = true
		}
	} else {
		s.Lines = append(s.Lines, Line{Text: *addition, Time: time.Now(), Type: lineType})
		if !endLine {
			setNewLine = true
			newLine = &s.Lines[len(s.Lines)-1]
		}
	}
	s.ContentBytes += len(*addition)
	s.clearOldLines()
	if s.Active {
		go s.Program.Send(ContentUpdateMsg{})
	}
	return
}

func (s *Service) addStdout(addition string, endLine bool) {
	if setNewLine, newLine := s.addOutput(&addition, endLine, s.currentStdoutLine, LineTypeStdout); setNewLine {
		s.currentStdoutLine = newLine
	}
}

func (s *Service) addSterr(addition string, endLine bool) {
	if setNewLine, newLine := s.addOutput(&addition, endLine, s.currentStderrLine, LineTypeStderr); setNewLine {
		s.currentStderrLine = newLine
	}
}

func (s *Service) addSyserrLine(addition string) {
	s.addOutput(&addition, true, nil, LineTypeSyserr)
}

func (s *Service) addSysoutLine(addition string) {
	s.addOutput(&addition, true, nil, LineTypeSysout)
}

func writeFromPipe(pipe *io.ReadCloser, isErrorPipe bool, s *Service, wg *sync.WaitGroup) {
	buf := bufio.NewReader(*pipe)
	for {
		line, isPrefix, err := buf.ReadLine()
		if err == io.EOF {
			wg.Done()
			return
		} else if err != nil {
			if err.Error() != "read |0: file already closed" {
				bufname := "stdout"
				if isErrorPipe {
					bufname = "stderr"
				}
				s.addSyserrLine(fmt.Sprintf("Error reading %s buffer: %s", bufname, err))
			}
			wg.Done()
			return
		} else {
			if isErrorPipe {
				s.addSterr(string(line), !isPrefix)
			} else {
				s.addStdout(string(line), !isPrefix)
			}
		}
	}
}
