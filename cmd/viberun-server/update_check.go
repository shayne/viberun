// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

type updateStatus struct {
	Available bool      `json:"available"`
	Current   string    `json:"current,omitempty"`
	Latest    string    `json:"latest,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
}

func startUpdateWatcher(app string, containerName string, interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		checkAndWriteUpdate(app, containerName)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				checkAndWriteUpdate(app, containerName)
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

func checkAndWriteUpdate(app string, containerName string) {
	status, err := checkImageUpdate(containerName)
	if err != nil {
		_ = writeUpdateStatus(app, updateStatus{Available: false, CheckedAt: time.Now().UTC()})
		return
	}
	_ = writeUpdateStatus(app, status)
}

func checkImageUpdate(containerName string) (updateStatus, error) {
	status := updateStatus{CheckedAt: time.Now().UTC()}
	imageRef := defaultImageRef()
	if err := runDockerCommandOutput("pull", imageRef); err != nil {
		return status, err
	}
	currentID, err := containerImageID(containerName)
	if err != nil {
		return status, err
	}
	latestID, err := imageID(imageRef)
	if err != nil {
		return status, err
	}
	status.Current = trimImageID(currentID)
	status.Latest = trimImageID(latestID)
	status.Available = currentID != "" && latestID != "" && currentID != latestID
	return status, nil
}

func imageID(name string) (string, error) {
	out, err := exec.Command("docker", "image", "inspect", "-f", "{{.Id}}", name).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func trimImageID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.TrimPrefix(value, "sha256:")
}

func writeUpdateStatus(app string, status updateStatus) error {
	cfg := hostRPCConfigForApp(app)
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	tmp := cfg.HostUpdateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, cfg.HostUpdateFile)
}

func clearUpdateStatus(app string) error {
	cfg := hostRPCConfigForApp(app)
	if _, err := os.Stat(cfg.HostUpdateFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(cfg.HostUpdateFile)
}
