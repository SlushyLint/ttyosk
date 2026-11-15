//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	jsEventButton = 0x01
	jsEventAxis   = 0x02
	jsEventInit   = 0x80
	jsAxisDead    = 16000
	jsButtonCount = 32
	jsAxisCount   = 8
)

type joystickDecoder struct {
	axisDirection [jsAxisCount]int
	buttonDown    [jsButtonCount]bool
}

func (d *joystickDecoder) decode(data []byte) (gamepadEvent, bool) {
	if len(data) != 8 {
		return gamepadEvent{}, false
	}
	eventType := data[6]
	if eventType&jsEventInit != 0 {
		return gamepadEvent{}, false
	}

	switch eventType {
	case jsEventAxis:
		axis := int(data[7])
		if axis >= len(d.axisDirection) {
			return gamepadEvent{}, false
		}
		value := int16(binary.NativeEndian.Uint16(data[4:6]))
		direction := 0
		if value > jsAxisDead {
			direction = 1
		} else if value < -jsAxisDead {
			direction = -1
		}
		if direction == d.axisDirection[axis] {
			return gamepadEvent{}, false
		}
		d.axisDirection[axis] = direction
		if direction == 0 {
			return gamepadEvent{}, false
		}
		switch axis {
		case 0, 6:
			return gamepadEvent{dx: direction}, true
		case 1, 7:
			return gamepadEvent{dy: direction}, true
		default:
			return gamepadEvent{}, false
		}
	case jsEventButton:
		button := int(data[7])
		if button >= len(d.buttonDown) {
			return gamepadEvent{}, false
		}
		pressed := data[4] != 0
		if pressed == d.buttonDown[button] {
			return gamepadEvent{}, false
		}
		d.buttonDown[button] = pressed
		if !pressed {
			return gamepadEvent{}, false
		}
		switch button {
		case 0:
			return gamepadEvent{selectKey: true}, true
		case 1:
			return gamepadEvent{backspace: true}, true
		case 12:
			return gamepadEvent{dy: -1}, true
		case 13:
			return gamepadEvent{dy: 1}, true
		case 14:
			return gamepadEvent{dx: -1}, true
		case 15:
			return gamepadEvent{dx: 1}, true
		default:
			return gamepadEvent{}, false
		}
	}
	return gamepadEvent{}, false
}

func watchGamepads(events chan<- gamepadEvent) {
	active := make(map[string]bool)
	closed := make(chan string)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	scan := func() {
		paths, err := filepath.Glob("/dev/input/js*")
		if err != nil {
			events <- gamepadEvent{err: fmt.Errorf("find joystick devices: %w", err)}
			return
		}
		for _, path := range paths {
			if active[path] {
				continue
			}
			file, err := os.Open(path)
			if err != nil {
				active[path] = true
				events <- gamepadEvent{err: fmt.Errorf("open %s: %w", path, err)}
				continue
			}
			active[path] = true
			go readJoystick(file, events, closed)
		}
	}

	scan()
	for {
		select {
		case path := <-closed:
			delete(active, path)
		case <-ticker.C:
			scan()
		}
	}
}

func readJoystick(file *os.File, events chan<- gamepadEvent, closed chan<- string) {
	path := file.Name()
	defer func() {
		_ = file.Close()
		closed <- path
	}()

	decoder := joystickDecoder{}
	data := make([]byte, 8)
	for {
		if _, err := io.ReadFull(file, data); err != nil {
			if !errors.Is(err, os.ErrClosed) {
				events <- gamepadEvent{err: fmt.Errorf("%s disconnected: %w", path, err)}
			}
			return
		}
		if event, ok := decoder.decode(data); ok {
			events <- event
		}
	}
}
