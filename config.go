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
	defaultListenAddr                  = ":12000"
	defaultMaxBodyBytes                = int64(20 << 20)
	defaultReadHeaderTimeout           = 30 * time.Second
	defaultServerIdleTimeout           = 90 * time.Second
	defaultUpstreamMaxIdleConns        = 200
	defaultUpstreamMaxIdleConnsPerHost = 50
	defaultUpstreamIdleConnTimeout     = 90 * time.Second
	defaultLogDir                      = "logs"
	defaultLogMaxDays                  = 7
)

type Config struct {
	ListenAddr                  string
	MaxBodyBytes                int64
	ReadHeaderTimeout           time.Duration
	ServerIdleTimeout           time.Duration
	UpstreamMaxIdleConns        int
	UpstreamMaxIdleConnsPerHost int
	UpstreamIdleConnTimeout     time.Duration
	LogDir                      string
	LogMaxDays                  int
	Projects                    []Project
}

type Project struct {
	Name              string
	UpstreamURL       *url.URL
	LangfusePublicKey string
	LangfuseSecretKey string
}

type fileConfig struct {
	Server   serverConfig    `yaml:"server"`
	Upstream upstreamConfig  `yaml:"upstream"`
	Log      logConfig       `yaml:"log"`
	Projects []projectConfig `yaml:"projects"`
}

type serverConfig struct {
	ListenAddr        string `yaml:"listen_addr"`
	MaxBodyBytes      int64  `yaml:"max_body_bytes"`
	ReadHeaderTimeout string `yaml:"read_header_timeout"`
	IdleTimeout       string `yaml:"idle_timeout"`
}

type upstreamConfig struct {
	MaxIdleConns        int    `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int    `yaml:"max_idle_conns_per_host"`
	IdleConnTimeout     string `yaml:"idle_conn_timeout"`
}

type logConfig struct {
	Dir     string `yaml:"dir"`
	MaxDays int    `yaml:"max_days"`
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
		ListenAddr:                  valueOrDefault(raw.Server.ListenAddr, defaultListenAddr),
		MaxBodyBytes:                defaultMaxBodyBytes,
		ReadHeaderTimeout:           defaultReadHeaderTimeout,
		ServerIdleTimeout:           defaultServerIdleTimeout,
		UpstreamMaxIdleConns:        defaultUpstreamMaxIdleConns,
		UpstreamMaxIdleConnsPerHost: defaultUpstreamMaxIdleConnsPerHost,
		UpstreamIdleConnTimeout:     defaultUpstreamIdleConnTimeout,
		LogDir:                      valueOrDefault(raw.Log.Dir, defaultLogDir),
		LogMaxDays:                  defaultLogMaxDays,
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
	if strings.TrimSpace(raw.Server.IdleTimeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(raw.Server.IdleTimeout))
		if err != nil || d <= 0 {
			return Config{}, errors.New("server.idle_timeout must be a positive duration")
		}
		cfg.ServerIdleTimeout = d
	}
	if raw.Upstream.MaxIdleConns > 0 {
		cfg.UpstreamMaxIdleConns = raw.Upstream.MaxIdleConns
	}
	if raw.Upstream.MaxIdleConns < 0 {
		return Config{}, errors.New("upstream.max_idle_conns must be a positive integer")
	}
	if raw.Upstream.MaxIdleConnsPerHost > 0 {
		cfg.UpstreamMaxIdleConnsPerHost = raw.Upstream.MaxIdleConnsPerHost
	}
	if raw.Upstream.MaxIdleConnsPerHost < 0 {
		return Config{}, errors.New("upstream.max_idle_conns_per_host must be a positive integer")
	}
	if strings.TrimSpace(raw.Upstream.IdleConnTimeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(raw.Upstream.IdleConnTimeout))
		if err != nil || d <= 0 {
			return Config{}, errors.New("upstream.idle_conn_timeout must be a positive duration")
		}
		cfg.UpstreamIdleConnTimeout = d
	}
	if raw.Log.MaxDays > 0 {
		cfg.LogMaxDays = raw.Log.MaxDays
	}
	if raw.Log.MaxDays < 0 {
		return Config{}, errors.New("log.max_days must be a positive integer")
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
