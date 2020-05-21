package main

// Forked from https://github.com/cosmo0920/fluent-bit-go-s3/blob/master/formatter.go

import (
	"bytes"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	ANSI_RESET   = "\033[0m"
	ANSI_BOLD    = "\033[1m"
	ANSI_CYAN    = "\033[96m"
	ANSI_MAGENTA = "\033[95m"
	ANSI_RED     = "\033[91m"
	ANSI_YELLOW  = "\033[93m"
	ANSI_BLUE    = "\033[94m"
	ANSI_GREEN   = "\033[92m"
	ANSI_WHITE   = "\033[97m"
)

type FluentBitLogFormat struct{}

// Format Specify logging format.
func (f *FluentBitLogFormat) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer

	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	headerTitle := ""

	headerColor := ""
	boldColor := ANSI_BOLD
	resetColor := ANSI_RESET

	switch entry.Level {
	case logrus.TraceLevel:
		headerTitle = "trace"
		headerColor = ANSI_BLUE
	case logrus.InfoLevel:
		headerTitle = "info"
		headerColor = ANSI_GREEN
	case logrus.WarnLevel:
		headerTitle = "warn"
		headerColor = ANSI_YELLOW
	case logrus.ErrorLevel:
		headerTitle = "error"
		headerColor = ANSI_RED
	case logrus.DebugLevel:
		headerTitle = "debug"
		headerColor = ANSI_YELLOW
	case logrus.FatalLevel:
		headerTitle = "fatal"
		headerColor = ANSI_MAGENTA
	}

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		headerColor = ""
		boldColor = ""
		resetColor = ""
	}

	time := fmt.Sprintf("%s[%s%s%s]%s",
		boldColor, resetColor,
		entry.Time.Format("2006/01/02 15:04:05"),
		boldColor, resetColor)
	b.WriteString(time)

	level := fmt.Sprintf(" [%s%5s%s] ", headerColor, headerTitle, resetColor)
	b.WriteString(level)

	if i, ok := entry.Data["interface"]; ok {
		b.WriteString(fmt.Sprintf("[%s] ", i))
	}

	if entry.Message != "" {
		b.WriteString(entry.Message)
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func NewLogger(i string, lvl logrus.Level) *logrus.Entry {
	l := logrus.New()
	l.Level = lvl
	l.SetFormatter(new(FluentBitLogFormat))

	return l.WithFields(logrus.Fields{"interface": i})
}
