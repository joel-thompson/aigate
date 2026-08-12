package secretsscan

import (
	"context"
	"strings"
	"testing"
)

func TestDetectsAWSAccessKey(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
--- a/config.go
+++ b/config.go
@@ -1,3 +1,4 @@
 package config
+var awsKey = "AKIAIOSFODNN7EXAMPLE"
 var x = 1`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for AWS key")
	}
	if !strings.Contains(result.Output, "AWS access key") {
		t.Errorf("expected output to mention AWS access key, got: %s", result.Output)
	}
}

func TestDetectsPrivateKeyBlock(t *testing.T) {
	diff := `diff --git a/key.pem b/key.pem
--- /dev/null
+++ b/key.pem
@@ -0,0 +1,3 @@
+-----BEGIN RSA PRIVATE KEY-----
+MIIEowIBAAKCAQEA...
+-----END RSA PRIVATE KEY-----`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for private key block")
	}
	if !strings.Contains(result.Output, "private key") {
		t.Errorf("expected output to mention private key, got: %s", result.Output)
	}
}

func TestDetectsNonRSAPrivateKeyBlock(t *testing.T) {
	diff := `diff --git a/key.pem b/key.pem
--- /dev/null
+++ b/key.pem
@@ -0,0 +1,3 @@
+-----BEGIN Ssh2 PRIVATE KEY-----
+body
+-----END Ssh2 PRIVATE KEY-----`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for non-RSA private key block")
	}
}

func TestDetectsGitHubPAT(t *testing.T) {
	diff := `diff --git a/auth.go b/auth.go
+++ b/auth.go
@@ -1 +1,2 @@
+token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"
 package auth`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for GitHub PAT")
	}
	if !strings.Contains(result.Output, "GitHub") {
		t.Errorf("expected output to mention GitHub, got: %s", result.Output)
	}
}

func TestDetectsOpenAIStripeKey(t *testing.T) {
	diff := `diff --git a/api.go b/api.go
+++ b/api.go
@@ -1 +1,2 @@
+apiKey := "sk-abcdefghij1234567890abcdef"
 package api`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for sk- key")
	}
	if !strings.Contains(result.Output, "API key") {
		t.Errorf("expected output to mention API key, got: %s", result.Output)
	}
}

func TestDetectsSlackTokenBot(t *testing.T) {
	diff := `diff --git a/slack.go b/slack.go
+++ b/slack.go
@@ -1 +1,2 @@
+var botToken = "xoxb-123456789012-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx"
 package slack`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for Slack bot token")
	}
	if !strings.Contains(result.Output, "Slack") {
		t.Errorf("expected output to mention Slack, got: %s", result.Output)
	}
}

func TestDetectsSlackTokenUser(t *testing.T) {
	diff := `diff --git a/slack.go b/slack.go
+++ b/slack.go
@@ -1 +1,2 @@
+var userToken = "xoxp-123456789012-1234567890123-1234567890123-abcdef1234567890abcdef1234567890"
 package slack`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for Slack user token")
	}
}

func TestReportsFileAndLineNumber(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
+++ b/config.go
@@ -1,3 +1,5 @@
 package config
+
+var awsKey = "AKIAIOSFODNN7EXAMPLE"
 var x = 1
 var y = 2`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "config.go:3") {
		t.Errorf("expected output to reference config.go:3, got: %s", result.Output)
	}
}

func TestMultipleSecretsInOneDiff(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
+++ b/config.go
@@ -1 +1,3 @@
+aws := "AKIAIOSFODNN7EXAMPLE"
+token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"
 package config`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(result.Output, "AWS") {
		t.Errorf("expected AWS finding, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "GitHub") {
		t.Errorf("expected GitHub finding, got: %s", result.Output)
	}
}

func TestCleanDiffPassesScan(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
+++ b/main.go
@@ -1 +1,3 @@
+func main() {
+    fmt.Println("hello")
+}
 package main`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for clean diff, got %d", result.ExitCode)
	}
}

func TestEmptyDiffPasses(t *testing.T) {
	p := &Provider{Diff: ""}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for empty diff, got %d", result.ExitCode)
	}
}

func TestIgnoresRemovedLines(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
+++ b/config.go
@@ -1,2 +1,1 @@
-var awsKey = "AKIAIOSFODNN7EXAMPLE"
 package config`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for removed secret, got %d", result.ExitCode)
	}
}

func TestIgnoresContextLines(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
+++ b/config.go
@@ -1,2 +1,3 @@
 var awsKey = "AKIAIOSFODNN7EXAMPLE"
+var x = 1
 package config`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 when secret is in context line only, got %d", result.ExitCode)
	}
}

func TestFalsePositive_Base64String(t *testing.T) {
	diff := `diff --git a/data.go b/data.go
+++ b/data.go
@@ -1 +1,2 @@
+var encoded = "SGVsbG8gV29ybGQhIFRoaXMgaXMgYSBiYXNlNjQgZW5jb2RlZCBzdHJpbmc="
 package data`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for base64 string (not a secret), got %d", result.ExitCode)
	}
}

func TestFalsePositive_LongHexHash(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
+++ b/main.go
@@ -1 +1,2 @@
+var commitHash = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
 package main`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for hex hash (not a secret), got %d", result.ExitCode)
	}
}

func TestFalsePositive_ShortSkPrefix(t *testing.T) {
	diff := `diff --git a/test_helpers.go b/test_helpers.go
+++ b/test_helpers.go
@@ -1 +1,2 @@
+var shortKey = "sk-short"
 package helpers`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for sk- prefix shorter than 20 chars, got %d", result.ExitCode)
	}
}

func TestLabel(t *testing.T) {
	diff := `diff --git a/config.go b/config.go
+++ b/config.go
@@ -1 +1,2 @@
+aws := "AKIAIOSFODNN7EXAMPLE"
 package config`
	p := &Provider{Diff: diff}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Label != "secrets-scan" {
		t.Errorf("expected label %q, got %q", "secrets-scan", result.Label)
	}
}
