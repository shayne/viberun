// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/shayne/viberun/internal/hostcmd"
	"github.com/shayne/viberun/internal/proxy"
)

func handleWipeCommand() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("viberun-server must run as root; run via sudo or re-run setup")
	}
	caddyName := proxy.DefaultCaddyContainer()
	if cfg, _, err := proxy.LoadConfig(); err == nil {
		if value := strings.TrimSpace(cfg.CaddyContainer); value != "" {
			caddyName = value
		}
	}
	toRemove := map[string]bool{}
	if _, err := exec.LookPath("docker"); err == nil {
		containers, err := listContainers()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		for _, name := range containers {
			if strings.HasPrefix(name, "viberun-") {
				toRemove[name] = true
			}
		}
		containersWithImages, err := listContainersWithImages()
		if err != nil {
			return fmt.Errorf("failed to list containers with images: %w", err)
		}
		for _, entry := range containersWithImages {
			if isViberunImage(containerImageRepo(entry.Image)) {
				toRemove[entry.Name] = true
			}
		}
		if strings.TrimSpace(caddyName) != "" {
			toRemove[caddyName] = true
		}
		names := make([]string, 0, len(toRemove))
		for name := range toRemove {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if err := runDockerCommandOutput("rm", "-f", "-v", name); err != nil {
				return err
			}
		}
		images, err := listDockerImages()
		if err != nil {
			return err
		}
		refs := imageRefsToRemove(images)
		for _, ref := range refs {
			if err := runDockerCommandOutput("rmi", "-f", ref); err != nil {
				return err
			}
		}
	}
	if err := cleanupHomeVolumes(); err != nil {
		return err
	}
	for _, path := range []string{
		"/var/lib/viberun",
		"/tmp/viberun-hostrpc",
		"/var/run/viberun-hostrpc",
		"/etc/viberun",
		"/etc/sudoers.d/viberun-server",
		"/usr/local/bin/viberun-server",
	} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func cleanupHomeVolumes() error {
	appsDir := "/var/lib/viberun/apps"
	entries, err := os.ReadDir(appsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read apps dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		_ = deleteHomeVolume(name)
	}
	unmountViberunMounts()
	return nil
}

func unmountViberunMounts() {
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		target := fields[1]
		if strings.HasPrefix(target, "/var/lib/viberun/apps/") {
			_ = hostcmd.Run("umount", target).Run()
		}
	}
}

type containerInfo struct {
	Name  string
	Image string
}

func listContainersWithImages() ([]containerInfo, error) {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}} {{.Image}}").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	containers := make([]containerInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		containers = append(containers, containerInfo{
			Name:  parts[0],
			Image: parts[1],
		})
	}
	return containers, nil
}

func containerImageRepo(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if at := strings.Index(image, "@"); at >= 0 {
		image = image[:at]
	}
	if colon := strings.LastIndex(image, ":"); colon > strings.LastIndex(image, "/") {
		image = image[:colon]
	}
	return image
}

type dockerImage struct {
	Repository string
	Tag        string
	ID         string
}

func listDockerImages() ([]dockerImage, error) {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}} {{.Tag}} {{.ID}}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	images := make([]dockerImage, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		images = append(images, dockerImage{
			Repository: parts[0],
			Tag:        parts[1],
			ID:         parts[2],
		})
	}
	return images, nil
}

func imageRefsToRemove(images []dockerImage) []string {
	seen := map[string]bool{}
	refs := make([]string, 0, len(images))
	for _, image := range images {
		repo := strings.TrimSpace(image.Repository)
		if repo == "" || repo == "<none>" {
			continue
		}
		if !isViberunImage(repo) {
			continue
		}
		ref := repo
		tag := strings.TrimSpace(image.Tag)
		if tag != "" && tag != "<none>" {
			ref = repo + ":" + tag
		} else if strings.TrimSpace(image.ID) != "" {
			ref = image.ID
		}
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func isViberunImage(repo string) bool {
	switch repo {
	case "viberun", "viberun-proxy":
		return true
	default:
		return strings.HasSuffix(repo, "/viberun") || strings.HasSuffix(repo, "/viberun-proxy")
	}
}
