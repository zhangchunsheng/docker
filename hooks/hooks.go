package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	registeredHooks = make(map[string][]*Hook)
)

type Hook struct {
	Name string // Filepath

	root     string // Root path
	fileName string
	hookName string
	action   string
}

func LoadAll(root string) error {
	return filepath.Walk(root, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fileInfo.IsDir() {
			p, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if err := NewHook(root, p); err != nil {
				return err
			}
		}
		return nil
	})
}

func NewHook(root, name string) error {
	if name[0] == '/' {
		name = name[1:]
	}
	h := &Hook{
		Name: name,
		root: root,
	}
	var action string
	parts := strings.Split(name, "/")
	h.hookName = parts[0]
	if len(parts) > 2 {
		action = parts[1]
	}
	h.action = action
	h.fileName = filepath.Base(name)

	hooks, exits := registeredHooks[h.hookName]
	if !exits {
		hooks = make([]*Hook, 0)
	}
	hooks = append(hooks, h)
	registeredHooks[h.hookName] = hooks

	return nil
}

func Execute(hook, action string, env []string) error {
	if hooks, exists := registeredHooks[hook]; exists {
		env = append(env, fmt.Sprintf("DOCKER_ACTION=%s_%s", hook, action))

		Sort(hooks)

		for _, h := range hooks {
			if h.action == "" || h.action == action {
				cmd := exec.Command(filepath.Join(h.root, h.Name))
				cmd.Env = env

				if err := cmd.Run(); err != nil {
					return fmt.Errorf("Hook failure: %s Error: %s", h.Name, err)
				}
			}
		}
	}
	return nil
}
