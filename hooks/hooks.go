package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	//TO REMOVE
	"github.com/dotcloud/docker/utils"
)

var (
	registeredHooks = make(map[string][]*Hook)
)

type Hook struct {
	root     string
	prefix   string
	category string
	action   string
	mode     string
	fileName string
}

func (h *Hook) Path() string {
	return filepath.Join(h.root, h.category, h.action, h.fileName)
}

func LoadAll(root, prefix string) error {
	err := filepath.Walk(root, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fileInfo.IsDir() && fileInfo.Mode()&0111 != 0 {
			p, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if err := NewHook(root, p, prefix); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func NewHook(root, name, prefix string) error {
	if name[0] == '/' {
		name = name[1:]
	}
	h := &Hook{
		root:   root,
		prefix: prefix,
	}
	var action, mode string
	parts := strings.Split(name, "/")
	h.category = parts[0]
	if len(parts) > 3 {
		mode = parts[2]
	}
	if len(parts) > 2 {
		action = parts[1]
	}
	h.action = action
	h.mode = mode
	h.fileName = filepath.Base(name)

	hooks, exists := registeredHooks[h.category]
	if !exists {
		hooks = make([]*Hook, 0)
	}
	hooks = append(hooks, h)
	registeredHooks[h.category] = hooks
	//TO REMOVE
	utils.Debugf("Registering a new hook [CATEGORY: %s] [ACTION: %s] [MODE: %s] %s", h.category, action, mode, h.fileName)
	return nil
}

func (h *Hook) executeWithTimeout(hook, action, mode string, env []string) error {
	c := make(chan error, 1)

	var cmd *exec.Cmd
	go func() {
		cmd = exec.Command(h.Path())
		env = append(env, fmt.Sprintf("%s_ACTION=%s_%s", h.prefix, hook, action))
		cmd.Env = append(env, fmt.Sprintf("%s_MODE=%s", h.prefix, mode))

		if err := cmd.Run(); err != nil {
			c <- fmt.Errorf("Hook failure: %s Error: %s", h.Path(), err)
		}
		c <- nil
	}()

	select {
	case err := <-c:
		if err != nil {
			return err
		}
	case <-time.After(2 * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		return fmt.Errorf("Hook failure: %s Error: timeout", h.Path())
	}
	return nil
}

func Execute(hook, action, mode string, env []string) error {
	if len(registeredHooks) == 0 {
		return nil
	}
	if hooks, exists := registeredHooks[hook]; exists {

		Sort(hooks)
		for _, h := range hooks {
			if h.action == "" || h.action == action && h.mode == "" || h.mode == mode {
				if mode == "pre" {
					return h.executeWithTimeout(hook, action, mode, env)
				} else if mode == "post" {
					go func() {
						// discard errors ?
						h.executeWithTimeout(hook, action, mode, env)
					}()
				}

			}
		}
	}
	return nil
}
