package applog

import (
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var Default = NewBuffer(1000)

type Entry struct {
	ID      int64  `json:"id"`
	Time    string `json:"time"`
	Message string `json:"message"`
}

type Buffer struct {
	mu      sync.Mutex
	limit   int
	nextID  int64
	partial string
	items   []Entry
}

func NewBuffer(limit int) *Buffer {
	if limit <= 0 {
		limit = 1000
	}
	return &Buffer{limit: limit}
}

func Install(limit int) {
	Default = NewBuffer(limit)
	log.SetOutput(io.MultiWriter(os.Stdout, Default))
	gin.DefaultWriter = io.MultiWriter(os.Stdout, Default)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, Default)
}

func Operation(action string, fields ...string) {
	action = safeToken(action)
	if action == "" {
		return
	}
	var builder strings.Builder
	builder.WriteString("operation action=")
	builder.WriteString(action)
	for index := 0; index+1 < len(fields); index += 2 {
		key := safeToken(fields[index])
		if key == "" {
			continue
		}
		builder.WriteByte(' ')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(strconv.Quote(safeField(fields[index+1])))
	}
	log.Print(builder.String())
}

func (b *Buffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	text := b.partial + string(value)
	lines := strings.Split(text, "\n")
	b.partial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		b.appendLocked(strings.TrimRight(line, "\r"))
	}
	return len(value), nil
}

func (b *Buffer) Entries(query string, limit int) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	query = strings.ToLower(strings.TrimSpace(query))
	if limit <= 0 || limit > b.limit {
		limit = b.limit
	}
	result := make([]Entry, 0, limit)
	for index := len(b.items) - 1; index >= 0 && len(result) < limit; index-- {
		item := b.items[index]
		if query != "" && !strings.Contains(strings.ToLower(item.Message), query) && !strings.Contains(strings.ToLower(item.Time), query) {
			continue
		}
		result = append(result, item)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = nil
	b.partial = ""
}

func (b *Buffer) appendLocked(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if strings.Contains(message, "/api/v1/settings/logs") {
		return
	}
	b.nextID++
	b.items = append(b.items, Entry{ID: b.nextID, Time: time.Now().Format(time.RFC3339), Message: message})
	if len(b.items) > b.limit {
		copy(b.items, b.items[len(b.items)-b.limit:])
		b.items = b.items[:b.limit]
	}
}

func safeToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}

func safeField(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	if len(value) > 200 {
		return value[:200] + "..."
	}
	return value
}
