// Package callcontrol coordinates door commands and one specific SIP call.
package callcontrol

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	Name string    `json:"name"`
}

type Calls interface {
	Current() string
	Reject(string) error
	Answer(context.Context, string) error
	Bye(context.Context, string) error
}

type Result struct {
	Opening string `json:"opening"`
	Call    string `json:"call"`
}

// Opening accepted is an upstream acknowledgement, not a door sensor reading.
func (r Result) Text() string {
	opening := map[string]string{"accepted": "Команда открытия принята", "failed": "Открытие не выполнено", "unknown": "Результат открытия неизвестен", "busy": "Открытие уже выполняется"}[r.Opening]
	call := map[string]string{"off": "Завершение звонка выключено", "absent": "Соответствующий звонок уже отсутствует", "ended": "Вызов завершён", "failed": "Не удалось завершить вызов", "not_attempted": "Завершение вызова не выполнялось"}[r.Call]
	return opening + ". " + call
}

type Controller struct {
	Calls    Calls
	Mode     string
	OpenDoor func(context.Context) error
	mu       sync.Mutex
}

// expected=nil captures the current call; a nonnil ID never falls back to a new call.
func (c *Controller) Open(ctx context.Context, expected *string) Result {
	if !c.mu.TryLock() {
		return Result{"busy", "not_attempted"}
	}
	defer c.mu.Unlock()
	id := ""
	if expected != nil {
		id = *expected
	} else if c.Calls != nil {
		id = c.Calls.Current()
	}
	active := id != "" && c.Calls != nil && c.Calls.Current() == id
	call := "absent"
	if c.Mode == "off" {
		call = "off"
	}
	answered := false
	if active && c.Mode == "answer-bye" {
		if err := c.Calls.Answer(ctx, id); err != nil {
			return Result{"failed", "failed"}
		}
		answered = true
	}
	err := c.OpenDoor(ctx)
	opening := "accepted"
	// A transport error can mean the command was already executed. Never retry it.
	if err != nil {
		opening = "unknown"
	}
	if answered {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		call = "ended"
		if c.Calls.Bye(cleanup, id) != nil {
			call = "failed"
		}
	} else if active && c.Mode == "reject" {
		call = "not_attempted"
		if err == nil {
			call = "ended"
			if c.Calls.Reject(id) != nil {
				call = "failed"
			}
		}
	}
	return Result{opening, call}
}
