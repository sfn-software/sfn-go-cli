package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const TermWidthDefault = 80
const TermHeightDefault = 24
const ProgressBarWidth = 22

type State uint8

const (
	Done      State = 0
	Failed    State = 1
	Warning   State = 2
	Sending   State = 3
	Receiving State = 4
)

type TtyColor string

const (
	ColorReset  TtyColor = "\033[0m"
	ColorRed    TtyColor = "\033[31m"
	ColorGreen  TtyColor = "\033[32m"
	ColorYellow TtyColor = "\033[33m"
	ColorBlue   TtyColor = "\033[34m"
	ColorPurple TtyColor = "\033[35m"
	ColorCyan   TtyColor = "\033[36m"
	ColorWhite  TtyColor = "\033[37m"
)

type TtyProgress struct {
	fileName string
	fileSize int64
	bytes    int64
	human    int64
	percent  uint
	state    State
	err      error
}

var ErrTtySizeInvalidFormat = errors.New("term: invalid format")

func GetTermSize() (uint, uint) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return TermWidthDefault, TermHeightDefault
	}
	width, height, err := parseTermSize(string(out))
	if err != nil {
		return TermWidthDefault, TermHeightDefault
	}
	return width, height
}

func parseTermSize(input string) (uint, uint, error) {
	parts := strings.Split(input, " ")
	if len(parts) != 2 {
		return 0, 0, ErrTtySizeInvalidFormat
	}
	height, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	width, err := strconv.Atoi(strings.Replace(parts[1], "\n", "", 1))
	if err != nil {
		return 0, 0, err
	}
	return uint(width), uint(height), nil
}

func NewProgress(fileName string, fileSize int64, direction State) *TtyProgress {
	return &TtyProgress{
		fileName: fileName,
		fileSize: fileSize,
		bytes:    0,
		human:    0,
		percent:  0,
		state:    direction,
	}
}

func (p *TtyProgress) Done() {
	p.state = Done
	p.percent = 100
	p.Draw(p.bytes)
}

func (p *TtyProgress) Failed(err error) {
	p.state = Failed
	p.err = err
	p.Draw(p.bytes)
}

func (p *TtyProgress) Warning(err error) {
	p.state = Warning
	p.err = err
	p.Draw(p.bytes)
}

func (p *TtyProgress) Draw(bytes int64) {
	p.bytes = bytes
	human, metrics := getSizeMetrics(bytes)
	percent := uint((bytes * 100) / p.fileSize)
	if p.human != human || percent != p.percent || p.state == Done || p.state == Failed || p.state == Warning {
		p.human = human
		p.percent = percent

		progress := getProgressBar(p.percent)
		ttyWidth, _ := GetTermSize()
		stateSymbol, stateColor := getStateAttrs(p.state)
		stateSymbol = Colored(stateSymbol, stateColor)

		switch p.state {
		case Failed, Warning:
			fileNameWithErr := fixedLengthString(p.fileName+" → "+p.err.Error(), int(ttyWidth-2))
			fmt.Printf("%s %s\r", stateSymbol, fileNameWithErr)
		default:
			fileName := fixedLengthString(p.fileName, int(ttyWidth-ProgressBarWidth-21))
			fmt.Printf("%s %s %4d %4s [%s] %3d %%\r", stateSymbol, fileName, human, metrics, progress, percent)
		}
	}
}

func getStateAttrs(direction State) (string, TtyColor) {
	switch direction {
	case Sending:
		return "↓", ColorReset
	case Receiving:
		return "↑", ColorReset
	case Done:
		return "✔", ColorGreen
	case Warning:
		return "↯", ColorYellow
	default:
		return "✘", ColorRed
	}
}

func getProgressBar(percent uint) (bar string) {
	for c := 0; c < ProgressBarWidth; c += 1 {
		if uint(c*100/ProgressBarWidth) <= percent {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return
}

func fixedLengthString(str string, length int) string {
	orgLen := len(str)
	if orgLen <= length {
		return str + strings.Repeat(" ", length-orgLen)
	}
	return "..." + str[orgLen-length+3:orgLen]
}

func getSizeMetrics(size int64) (int64, string) {
	var metrics string
	if size < 1024 {
		metrics = "Byte"
	}
	if size >= 1024 {
		size /= 1024
		metrics = "KiB "
	}
	if size >= 1024 {
		size /= 1024
		metrics = "MiB "
	}
	if size >= 1024 {
		size /= 1024
		metrics = "GiB "
	}
	if size >= 1024 {
		size /= 1024
		metrics = "TiB "
	}
	return size, metrics
}

func Colored(format string, color TtyColor, a ...any) string {
	return fmt.Sprintf(string(color)+format+string(ColorReset), a...)
}
