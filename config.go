package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListenAddr        = ":12000"
	defaultMaxBodyBytes      = int64(20 << 20)
	defaultReadHeaderTimeout = 10 * time.Second
)

type Config struct {
	ListenAddr        string
	MaxBodyBytes      int64
	ReadHeaderTimeout time.Duration
	Projects          []Project
}

type Project struct {
	Name              string
	UpstreamURL       *url.URL
	LangfusePublicKey string
	LangfuseSecretKey string
}

type fileConfig struct {
	Server   serverConfig    `yaml:"server"`
	Projects []projectConfig `yaml:"projects"`
}

type serverConfig struct {
	ListenAddr        string `yaml:"listen_addr"`
	MaxBodyBytes      int64  `yaml:"max_body_bytes"`
	ReadHeaderTimeout string `yaml:"read_header_timeout"`
}

type projectConfig struct {
	Name              string `yaml:"name"`
	LangfuseBaseURL   string `yaml:"langfuse_base_url"`
	LangfusePublicKey string `yaml:"langfuse_public_key"`
	LangfuseSecretKey string `yaml:"langfuse_secret_key"`
}

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	return LoadYAML(data)
}

func LoadYAML(data []byte) (Config, error) {
	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse yaml config: %w", err)
	}

	cfg := Config{
		ListenAddr:        valueOrDefault(raw.Server.ListenAddr, defaultListenAddr),
		MaxBodyBytes:      defaultMaxBodyBytes,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	if raw.Server.MaxBodyBytes > 0 {
		cfg.MaxBodyBytes = raw.Server.MaxBodyBytes
	}
	if strings.TrimSpace(raw.Server.ReadHeaderTimeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(raw.Server.ReadHeaderTimeout))
		if err != nil || d <= 0 {
			return Config{}, errors.New("server.read_header_timeout must be a positive duration")
		}
		cfg.ReadHeaderTimeout = d
	}

	if len(raw.Projects) == 0 {
		return Config{}, errors.New("at least one project is required")
	}

	seenPublicKeys := make(map[string]struct{}, len(raw.Projects))
	for i, item := range raw.Projects {
		project, err := parseProject(i, item)
		if err != nil {
			return Config{}, err
		}
		if _, ok := seenPublicKeys[project.LangfusePublicKey]; ok {
			return Config{}, fmt.Errorf("projects[%d].langfuse_public_key is duplicated", i)
		}
		seenPublicKeys[project.LangfusePublicKey] = struct{}{}
		cfg.Projects = append(cfg.Projects, project)
	}

	return cfg, nil
}

func parseProject(index int, item projectConfig) (Project, error) {
	project := Project{
		Name:              strings.TrimSpace(item.Name),
		LangfusePublicKey: strings.TrimSpace(item.LangfusePublicKey),
		LangfuseSecretKey: strings.TrimSpace(item.LangfuseSecretKey),
	}

	if project.Name == "" {
		project.Name = fmt.Sprintf("project-%d", index+1)
	}
	if project.LangfusePublicKey == "" {
		return Project{}, fmt.Errorf("projects[%d].langfuse_public_key is required", index)
	}
	if project.LangfuseSecretKey == "" {
		return Project{}, fmt.Errorf("projects[%d].langfuse_secret_key is required", index)
	}

	upstreamRaw := strings.TrimSpace(item.LangfuseBaseURL)
	if upstreamRaw == "" {
		return Project{}, fmt.Errorf("projects[%d].langfuse_base_url is required", index)
	}
	upstream, err := url.Parse(upstreamRaw)
	if err != nil {
		return Project{}, fmt.Errorf("projects[%d].langfuse_base_url is invalid: %w", index, err)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return Project{}, fmt.Errorf("projects[%d].langfuse_base_url must start with http:// or https://", index)
	}
	if upstream.Host == "" {
		return Project{}, fmt.Errorf("projects[%d].langfuse_base_url must include a host", index)
	}
	upstream.Path = strings.TrimRight(upstream.Path, "/")
	project.UpstreamURL = upstream

	return project, nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
