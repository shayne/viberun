// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package agents

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

//go:embed agents.json
var agentsData []byte

type Definition struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Command     []string `json:"command"`
}

type Catalog struct {
	Agents []Definition `json:"agents"`
}

type Spec struct {
	Provider string
	ID       string
	Label    string
	Command  []string
	Builtin  bool
}

var (
	builtinsOnce sync.Once
	builtins     []Definition
	builtinsErr  error
)

func Builtins() ([]Definition, error) {
	builtinsOnce.Do(func() {
		var catalog Catalog
		if err := json.Unmarshal(agentsData, &catalog); err != nil {
			builtinsErr = fmt.Errorf("failed to parse agents catalog: %w", err)
			return
		}
		if len(catalog.Agents) == 0 {
			builtinsErr = errors.New("agents catalog is empty")
			return
		}
		for i := range catalog.Agents {
			catalog.Agents[i].ID = strings.ToLower(strings.TrimSpace(catalog.Agents[i].ID))
			catalog.Agents[i].Label = strings.TrimSpace(catalog.Agents[i].Label)
			if catalog.Agents[i].ID == "" {
				builtinsErr = errors.New("agents catalog contains empty id")
				return
			}
			if len(catalog.Agents[i].Command) == 0 {
				builtinsErr = fmt.Errorf("agent %q has empty command", catalog.Agents[i].ID)
				return
			}
		}
		builtins = catalog.Agents
	})
	if builtinsErr != nil {
		return nil, builtinsErr
	}
	return append([]Definition(nil), builtins...), nil
}

func DefaultProvider() string {
	defs, err := Builtins()
	if err != nil || len(defs) == 0 {
		return "codex"
	}
	return defs[0].ID
}

func Resolve(provider string) (Spec, error) {
	resolved := strings.TrimSpace(provider)
	if resolved == "" {
		resolved = DefaultProvider()
	}
	lowered := strings.ToLower(resolved)
	defs, err := Builtins()
	if err != nil {
		return Spec{}, err
	}
	for _, def := range defs {
		if lowered == def.ID || matchesAlias(lowered, def.Aliases) {
			label := def.Label
			if label == "" {
				label = def.ID
			}
			return Spec{
				Provider: def.ID,
				ID:       def.ID,
				Label:    label,
				Command:  []string{def.ID},
				Builtin:  true,
			}, nil
		}
	}
	if strings.HasPrefix(lowered, "npx:") {
		pkg := strings.TrimSpace(resolved[len("npx:"):])
		if pkg == "" {
			return Spec{}, errors.New("npx agent requires a package name")
		}
		return Spec{
			Provider: "npx:" + pkg,
			Label:    labelFromPackage(pkg),
			Command:  []string{"npx", "-y", pkg},
		}, nil
	}
	if strings.HasPrefix(lowered, "uvx:") {
		pkg := strings.TrimSpace(resolved[len("uvx:"):])
		if pkg == "" {
			return Spec{}, errors.New("uvx agent requires a package name")
		}
		return Spec{
			Provider: "uvx:" + pkg,
			Label:    labelFromPackage(pkg),
			Command:  []string{"uvx", pkg},
		}, nil
	}
	return Spec{}, fmt.Errorf("unsupported provider %q", provider)
}

func matchesAlias(provider string, aliases []string) bool {
	if len(aliases) == 0 {
		return false
	}
	for _, alias := range aliases {
		if provider == strings.ToLower(strings.TrimSpace(alias)) {
			return true
		}
	}
	return false
}

func labelFromPackage(pkg string) string {
	trimmed := strings.TrimSpace(pkg)
	if trimmed == "" {
		return "agent"
	}
	if strings.HasPrefix(trimmed, "@") {
		trimmed = strings.TrimPrefix(trimmed, "@")
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 2 {
			trimmed = parts[1]
		}
	}
	if idx := strings.LastIndex(trimmed, "@"); idx > 0 {
		trimmed = trimmed[:idx]
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "="} {
		if idx := strings.Index(trimmed, sep); idx > 0 {
			trimmed = trimmed[:idx]
			break
		}
	}
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "agent"
	}
	return trimmed
}
