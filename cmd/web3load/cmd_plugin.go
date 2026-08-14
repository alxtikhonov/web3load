package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/web3load/web3load/internal/action"
	"github.com/web3load/web3load/internal/plugin"
)

// loadPlugins starts one subprocess per --plugin name=path spec and
// registers each as an action under "plugin:<name>". It returns every
// process it managed to start even when a later spec fails, so the
// caller's cleanup can still close the ones that did start rather than
// leaking them.
func loadPlugins(specs []string) ([]*plugin.Process, error) {
	var processes []*plugin.Process
	for _, spec := range specs {
		name, path, ok := strings.Cut(spec, "=")
		if !ok || name == "" || path == "" {
			return processes, fmt.Errorf("invalid --plugin %q, want name=path", spec)
		}
		proc, err := plugin.Start(path)
		if err != nil {
			return processes, fmt.Errorf("starting plugin %q: %w", name, err)
		}
		action.RegisterPlugin("plugin:"+name, proc)
		processes = append(processes, proc)
		slog.Info("plugin loaded", "name", name, "path", path)
	}
	return processes, nil
}

func closePlugins(processes []*plugin.Process) {
	for _, p := range processes {
		if err := p.Close(); err != nil {
			slog.Warn("plugin process did not exit cleanly", "error", err)
		}
	}
}
