package redstone

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type Logger struct {
	Service string
}

func NewLogger(service string) *Logger {
	return &Logger{Service: service}
}

func (l *Logger) Info(msg string, fields map[string]any) {
	l.emit("INFO", msg, fields)
}

func (l *Logger) Error(msg string, fields map[string]any) {
	l.emit("ERROR", msg, fields)
}

func (l *Logger) emit(level, msg string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["level"] = level
	fields["msg"] = msg
	fields["service"] = l.Service
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	b, _ := json.Marshal(fields)
	log.New(os.Stdout, "", 0).Println(string(b))
}
