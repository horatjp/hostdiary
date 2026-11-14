// Package config reads the single strict JSON configuration without side effects.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Filter struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}
type Config struct {
	Hostname string              `json:"hostname"`
	Apps     map[string][]string `json:"apps"`
	Net      Filter              `json:"net"`
	Disk     Filter              `json:"disk"`
	DiskIO   Filter              `json:"diskio"`
	Dirs     Dirs                `json:"dirs"`
}

type Dirs struct {
	Enabled *bool `json:"enabled"`
}

func (d Dirs) IsEnabled() bool { return d.Enabled == nil || *d.Enabled }

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	// Decode through a pointer so the JSON null is rejected, rather than defaulted.
	var p *Config
	if err = d.Decode(&p); err != nil {
		return c, err
	}
	if p == nil {
		return c, fmt.Errorf("config must be an object")
	}
	c = *p
	if err = d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("config must contain one JSON object")
	}
	for name, aliases := range c.Apps {
		if strings.TrimSpace(name) == "" || len(aliases) == 0 {
			return c, fmt.Errorf("app needs a name and comm aliases")
		}
		for _, alias := range aliases {
			if alias == "" {
				return c, fmt.Errorf("empty app comm")
			}
			if len(alias) > 15 {
				return c, fmt.Errorf("app %q: comm is truncated to 15 bytes; aliases must be at most 15 bytes", name)
			}
		}
	}
	return c, nil
}

// Select uses include as an allowlist; explicit excludes always win.
func (f Filter) Select(name string, defaultExcluded bool) bool {
	contains := func(xs []string) bool {
		for _, x := range xs {
			if x == name {
				return true
			}
		}
		return false
	}
	if contains(f.Exclude) {
		return false
	}
	if len(f.Include) > 0 {
		return contains(f.Include)
	}
	return !defaultExcluded
}
