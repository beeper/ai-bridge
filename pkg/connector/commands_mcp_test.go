package connector

import (
	"strings"
	"testing"
)

func TestParseMCPAddArgsHTTPDefault(t *testing.T) {
	name, cfg, err := parseMCPAddArgs([]string{"docs", "https://mcp.example.com", "tok123", "bearer"}, true)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if name != "docs" {
		t.Fatalf("expected name docs, got %q", name)
	}
	if cfg.Transport != mcpTransportStreamableHTTP {
		t.Fatalf("expected transport %q, got %q", mcpTransportStreamableHTTP, cfg.Transport)
	}
	if cfg.Endpoint != "https://mcp.example.com" {
		t.Fatalf("expected endpoint https://mcp.example.com, got %q", cfg.Endpoint)
	}
	if cfg.Token != "tok123" {
		t.Fatalf("expected token tok123, got %q", cfg.Token)
	}
	if cfg.AuthType != "bearer" {
		t.Fatalf("expected auth_type bearer, got %q", cfg.AuthType)
	}
}

func TestParseMCPAddArgsHTTPExplicitTransport(t *testing.T) {
	name, cfg, err := parseMCPAddArgs([]string{"docs", "streamable_http", "https://mcp.example.com", "tok123", "apikey"}, true)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if name != "docs" {
		t.Fatalf("expected name docs, got %q", name)
	}
	if cfg.Transport != mcpTransportStreamableHTTP {
		t.Fatalf("expected transport %q, got %q", mcpTransportStreamableHTTP, cfg.Transport)
	}
	if cfg.Endpoint != "https://mcp.example.com" {
		t.Fatalf("expected endpoint https://mcp.example.com, got %q", cfg.Endpoint)
	}
	if cfg.AuthType != "apikey" {
		t.Fatalf("expected auth_type apikey, got %q", cfg.AuthType)
	}
}

func TestParseMCPAddArgsStdio(t *testing.T) {
	name, cfg, err := parseMCPAddArgs([]string{"local", "stdio", "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, true)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if name != "local" {
		t.Fatalf("expected name local, got %q", name)
	}
	if cfg.Transport != mcpTransportStdio {
		t.Fatalf("expected transport %q, got %q", mcpTransportStdio, cfg.Transport)
	}
	if cfg.Command != "npx" {
		t.Fatalf("expected command npx, got %q", cfg.Command)
	}
	if len(cfg.Args) != 3 {
		t.Fatalf("expected 3 command args, got %d", len(cfg.Args))
	}
	if cfg.AuthType != "none" {
		t.Fatalf("expected auth_type none for stdio, got %q", cfg.AuthType)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("expected empty endpoint for stdio, got %q", cfg.Endpoint)
	}
}

func TestParseMCPAddArgsStdioDisabled(t *testing.T) {
	_, _, err := parseMCPAddArgs([]string{"local", "stdio", "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, false)
	if err == nil || err.Error() != "stdio disabled" {
		t.Fatalf("expected stdio disabled error, got: %v", err)
	}
}

func TestMCPUsageHidesStdioWhenDisabled(t *testing.T) {
	if strings.Contains(mcpAddUsage(false), "stdio") {
		t.Fatalf("expected stdio to be absent from add usage when disabled")
	}
	if strings.Contains(mcpManageUsage(false), "stdio") {
		t.Fatalf("expected stdio to be absent from manage usage when disabled")
	}
}
