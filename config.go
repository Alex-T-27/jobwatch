package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type target struct {
	Vendor  string `json:"vendor"`
	Company string `json:"company"`
}

type config struct {
	Targets []target `json:"targets"`
}

func loadTargets(path string) ([]target, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open company config %q: %w", path, err)
	}
	defer file.Close()

	var cfg config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode company config %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("company config %q must contain one JSON object", path)
	}

	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("company config %q has no targets", path)
	}

	seen := make(map[string]bool, len(cfg.Targets))
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		t.Vendor = strings.ToLower(strings.TrimSpace(t.Vendor))
		t.Company = strings.TrimSpace(t.Company)

		if t.Vendor == "" || t.Company == "" {
			return nil, fmt.Errorf("company config %q target %d requires vendor and company", path, i+1)
		}

		switch t.Vendor {
		case "ashby", "greenhouse", "lever":
		default:
			return nil, fmt.Errorf("company config %q target %d has unsupported vendor %q", path, i+1, t.Vendor)
		}

		key := t.Vendor + ":" + t.Company
		if seen[key] {
			return nil, fmt.Errorf("company config %q contains duplicate target %q", path, key)
		}
		seen[key] = true
	}

	return cfg.Targets, nil
}
