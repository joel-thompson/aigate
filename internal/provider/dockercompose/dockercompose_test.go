package dockercompose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joelthompson/aigate/internal/provider"
)

const configKey = "compose config --format json"
const psKey = "compose ps --format json"

func makeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// composeRoot is a temp dir holding the minimum that gets past the
// filesystem gate. The file's contents are never parsed by aigate — the
// stubbed CLI supplies the stack — so an empty file is enough.
func composeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"docker-compose.yml": ""})
	return root
}

// stubCLI dispatches on the joined args, following ciinfo's cli_test.go: a
// plain closure rather than a mock framework.
func stubCLI(out map[string]string, errs map[string]error) cliRunner {
	return func(_ context.Context, _ string, args ...string) (string, error) {
		key := strings.TrimSpace(strings.Join(args, " "))
		if err, ok := errs[key]; ok {
			return "", err
		}
		return out[key], nil
	}
}

// failingCLI stands in for a missing docker binary, so tests never depend on
// whether the machine running them has docker installed.
func failingCLI(_ context.Context, _ string, args ...string) (string, error) {
	return "", errors.New("docker: command not found")
}

func run(t *testing.T, p *Provider) string {
	t.Helper()
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Label != label {
		t.Errorf("expected label %q, got %q", label, result.Label)
	}
	return result.Output
}

// twoServiceConfig is one running (postgres) and one not (migrator).
const twoServiceConfig = `{
  "name": "demo",
  "services": {
    "postgres": {
      "image": "postgres:16",
      "ports": [{"mode":"ingress","host_ip":"127.0.0.1","target":5432,"published":"5432","protocol":"tcp"}]
    },
    "migrator": {
      "build": {"context": "%s/migrator"},
      "ports": null
    }
  }
}`

const postgresRunningPS = `{"Service":"postgres","State":"running","Health":"healthy","Image":"postgres:16","Publishers":[{"URL":"127.0.0.1","TargetPort":5432,"PublishedPort":5432,"Protocol":"tcp"}]}`

func TestRun_NoComposeFile(t *testing.T) {
	var calls [][]string
	recordCLI := func(_ context.Context, _ string, args ...string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	p := &Provider{Root: t.TempDir(), runCLI: recordCLI}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output with no compose file, got %q", got)
	}
	if len(calls) != 0 {
		t.Errorf("expected zero CLI calls with no compose file, got %d: %v", len(calls), calls)
	}
}

func TestRun_ComposeFileButDockerMissing(t *testing.T) {
	p := &Provider{Root: composeRoot(t), runCLI: failingCLI}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output when docker is missing, got %q", got)
	}
}

func TestRun_ConfigReturnsInvalidJSON(t *testing.T) {
	p := &Provider{Root: composeRoot(t), runCLI: stubCLI(map[string]string{configKey: "not json"}, nil)}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output for unparseable config, got %q", got)
	}
}

func TestRun_GroupsRunningAndNotRunning(t *testing.T) {
	root := composeRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     postgresRunningPS,
	}, nil)}

	got := run(t, p)
	want := `docker-compose.yml · project "demo" · 2 services
Start: docker compose up -d   (one service: docker compose up -d <name>)

Running (1/2) — already up, do not restart:
  postgres  postgres:16       127.0.0.1:5432→5432  healthy

Not running (1/2):
  migrator  build ./migrator  no published ports`
	if got != want {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}
}

func TestRun_DaemonUnreachableCollapsesToOneList(t *testing.T) {
	root := composeRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(
		map[string]string{configKey: fmt.Sprintf(twoServiceConfig, root)},
		map[string]error{psKey: errors.New("cannot connect to the Docker daemon")},
	)}

	got := run(t, p)
	if !strings.Contains(got, "Docker daemon not reachable; cannot tell which services are already running.") {
		t.Errorf("expected the daemon disclaimer, got:\n%s", got)
	}
	if !strings.Contains(got, "Services:") {
		t.Errorf("expected a single Services: list, got:\n%s", got)
	}
	if strings.Contains(got, "Not running") {
		t.Errorf("expected no not-running claim when ps failed, got:\n%s", got)
	}
	if strings.Contains(got, "Running (") {
		t.Errorf("expected no running claim when ps failed, got:\n%s", got)
	}
	if !strings.Contains(got, "postgres:16") {
		t.Errorf("expected the stack to still be described, got:\n%s", got)
	}
}

func TestRun_RunningPortsComeFromPsNotConfig(t *testing.T) {
	root := composeRoot(t)
	ps := `{"Service":"postgres","State":"running","Image":"postgres:16","Publishers":[{"URL":"127.0.0.1","TargetPort":5432,"PublishedPort":15432,"Protocol":"tcp"}]}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     ps,
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "127.0.0.1:15432→5432") {
		t.Errorf("expected the live published port 15432 from ps, got:\n%s", got)
	}
	if strings.Contains(got, "127.0.0.1:5432→5432") {
		t.Errorf("expected the stale config port not to be used for a running service, got:\n%s", got)
	}
}

func TestRun_ServiceWithNoPorts(t *testing.T) {
	root := composeRoot(t)
	cfg := `{"name":"demo","services":{"worker":{"image":"busybox","ports":null}}}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ""}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "no published ports") {
		t.Errorf("expected \"no published ports\", got:\n%s", got)
	}
}

func TestRun_PortRangeAndRandomAndUDP(t *testing.T) {
	root := composeRoot(t)
	cfg := `{"name":"demo","services":{"svc":{"image":"busybox","ports":[
		{"target":8000,"published":"8000-8002","protocol":"tcp"},
		{"target":9000,"published":"","protocol":"tcp"},
		{"host_ip":"127.0.0.1","target":8125,"published":"8125","protocol":"udp"},
		{"host_ip":"0.0.0.0","target":80,"published":"80","protocol":"tcp"}]}}}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ""}, nil)}

	got := run(t, p)
	for _, want := range []string{"8000-8002→8000", "→9000 (random host port)", "127.0.0.1:8125→8125/udp", "80→80"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "0.0.0.0") {
		t.Errorf("expected the 0.0.0.0 wildcard host to be dropped, got:\n%s", got)
	}
}

func TestRun_BuildContextRenderedRelativeToRoot(t *testing.T) {
	root := composeRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     "",
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "build ./migrator") {
		t.Errorf("expected a relative build context, got:\n%s", got)
	}
	if strings.Contains(got, root) {
		t.Errorf("expected the absolute root path %q to be absent, got:\n%s", root, got)
	}
}

func TestRun_HealthAppendedWhenPresent(t *testing.T) {
	root := composeRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     postgresRunningPS,
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "127.0.0.1:5432→5432  healthy") {
		t.Errorf("expected health appended after the ports, got:\n%s", got)
	}
}

func TestRun_NonRunningStateShown(t *testing.T) {
	root := composeRoot(t)
	ps := `{"Service":"postgres","State":"restarting","Health":"unhealthy","Image":"postgres:16","Publishers":[]}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     ps,
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "restarting") {
		t.Errorf("expected the non-running state to be shown, got:\n%s", got)
	}
}

func TestRun_RunningServiceAbsentFromConfigStillListed(t *testing.T) {
	root := composeRoot(t)
	cfg := `{"name":"demo","services":{"postgres":{"image":"postgres:16","ports":null}}}`
	ps := postgresRunningPS + "\n" +
		`{"Service":"debug-tools","State":"running","Image":"busybox:latest","Publishers":[{"URL":"127.0.0.1","TargetPort":22,"PublishedPort":2222,"Protocol":"tcp"}]}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ps}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "debug-tools") {
		t.Errorf("expected a profiled running service absent from config to still be listed, got:\n%s", got)
	}
	if !strings.Contains(got, "busybox:latest") {
		t.Errorf("expected the ps image for a config-less service, got:\n%s", got)
	}
	if !strings.Contains(got, "Running (2/2)") {
		t.Errorf("expected the config-less service counted in the total, got:\n%s", got)
	}
}

// TestRun_HeaderCountsOnlyTheDefaultStack guards the promise the Start line
// makes: `up -d` starts the compose file's services, not whatever else
// happens to be running, so the two must not share one count.
func TestRun_HeaderCountsOnlyTheDefaultStack(t *testing.T) {
	root := composeRoot(t)
	cfg := `{"name":"demo","services":{"postgres":{"image":"postgres:16","ports":null}}}`
	ps := postgresRunningPS + "\n" +
		`{"Service":"debug-tools","State":"running","Image":"busybox:latest","Publishers":[]}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ps}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "1 service (+1 running outside it)") {
		t.Errorf("expected the profiled extra disclosed outside the stack count, got:\n%s", got)
	}
	if strings.Contains(got, "· 2 services") {
		t.Errorf("expected up -d not to be credited with starting the extra, got:\n%s", got)
	}
}

func TestRun_HeaderOmitsExtrasWhenStackAccountsForEverything(t *testing.T) {
	root := composeRoot(t)
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{
		configKey: fmt.Sprintf(twoServiceConfig, root),
		psKey:     postgresRunningPS,
	}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "· 2 services") {
		t.Errorf("expected a plain stack count, got:\n%s", got)
	}
	if strings.Contains(got, "running outside it") {
		t.Errorf("expected no extras clause when there are none, got:\n%s", got)
	}
}

// TestRun_PaddingIgnoresTruncatedRows pins the column widths to what is
// actually printed: a very long name past the cap must not open a gutter in
// front of every visible row.
func TestRun_PaddingIgnoresTruncatedRows(t *testing.T) {
	root := composeRoot(t)
	var svcs []string
	for i := 0; i < maxServices; i++ {
		svcs = append(svcs, fmt.Sprintf(`"svc-%02d":{"image":"busybox","ports":null}`, i))
	}
	longName := "zz-service-name-far-past-the-cap"
	svcs = append(svcs, fmt.Sprintf(`"%s":{"image":"busybox","ports":null}`, longName))
	cfg := `{"name":"demo","services":{` + strings.Join(svcs, ",") + `}}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ""}, nil)}

	got := run(t, p)
	if strings.Contains(got, longName) {
		t.Fatalf("precondition: expected %q to be truncated away, got:\n%s", longName, got)
	}
	if !strings.Contains(got, "  svc-00  busybox") {
		t.Errorf("expected padding sized to the printed rows only, got:\n%s", got)
	}
}

func TestRun_EnvironmentNeverPrinted(t *testing.T) {
	root := composeRoot(t)
	cfg := `{"name":"demo","services":{"postgres":{"image":"postgres:16","environment":{"POSTGRES_PASSWORD":"s3cr3t"},"ports":null}}}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ""}, nil)}

	got := run(t, p)
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("expected the resolved environment value to be absent, got:\n%s", got)
	}
	if strings.Contains(got, "POSTGRES_PASSWORD") {
		t.Errorf("expected the environment key to be absent, got:\n%s", got)
	}
}

func TestRun_TruncatesOverMaxServices(t *testing.T) {
	root := composeRoot(t)
	var svcs []string
	for i := 0; i < maxServices+5; i++ {
		svcs = append(svcs, fmt.Sprintf(`"svc-%02d":{"image":"busybox","ports":null}`, i))
	}
	cfg := `{"name":"demo","services":{` + strings.Join(svcs, ",") + `}}`
	p := &Provider{Root: root, runCLI: stubCLI(map[string]string{configKey: cfg, psKey: ""}, nil)}

	got := run(t, p)
	if !strings.Contains(got, "... (5 more)") {
		t.Errorf("expected a truncation line, got:\n%s", got)
	}
	if strings.Contains(got, "svc-31") {
		t.Errorf("expected services past the cap to be elided, got:\n%s", got)
	}
}

func TestRun_StartCommandIncludesConfiguredFiles(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"a.yml": "", "b.yml": ""})

	var calls [][]string
	recordCLI := func(_ context.Context, _ string, args ...string) (string, error) {
		calls = append(calls, args)
		if strings.Contains(strings.Join(args, " "), "config") {
			return `{"name":"demo","services":{"svc":{"image":"busybox","ports":null}}}`, nil
		}
		return "", nil
	}
	p := &Provider{Root: root, Files: []string{"a.yml", "b.yml"}, runCLI: recordCLI}

	got := run(t, p)
	if !strings.Contains(got, "Start: docker compose -f a.yml -f b.yml up -d") {
		t.Errorf("expected the configured files in the start command, got:\n%s", got)
	}
	if len(calls) != 2 {
		t.Fatalf("expected two CLI calls, got %d: %v", len(calls), calls)
	}
	wantPrefix := "compose -f a.yml -f b.yml"
	for _, args := range calls {
		joined := strings.Join(args, " ")
		if !strings.HasPrefix(joined, wantPrefix) {
			t.Errorf("expected probe %q to use prefix %q", joined, wantPrefix)
		}
	}
}

func TestRun_ConfiguredFileMissingSkipsEntirely(t *testing.T) {
	var calls [][]string
	recordCLI := func(_ context.Context, _ string, args ...string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	root := composeRoot(t)
	p := &Provider{Root: root, Files: []string{"absent.yml"}, runCLI: recordCLI}

	got := run(t, p)
	if got != "" {
		t.Errorf("expected empty output when a configured file is missing, got %q", got)
	}
	if len(calls) != 0 {
		t.Errorf("expected zero CLI calls when a configured file is missing, got %d: %v", len(calls), calls)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	var calls [][]string
	recordCLI := func(_ context.Context, _ string, args ...string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	p := &Provider{Root: composeRoot(t), runCLI: recordCLI}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "" {
		t.Errorf("expected empty output for a cancelled context, got %q", result.Output)
	}
	if len(calls) != 0 {
		t.Errorf("expected zero CLI calls for a cancelled context, got %d: %v", len(calls), calls)
	}
}

func TestRun_ReturnsResultNotNil(t *testing.T) {
	p := &Provider{Root: t.TempDir(), runCLI: failingCLI}

	var result *provider.Result
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a non-nil result")
	}
}

func TestDecodePS_NDJSON(t *testing.T) {
	out := postgresRunningPS + "\n" + `{"Service":"redis","State":"running","Image":"redis:8","Publishers":[]}` + "\n"

	svcs, err := decodePS(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	if svcs[0].Service != "postgres" {
		t.Errorf("expected %q, got %q", "postgres", svcs[0].Service)
	}
	if svcs[1].Service != "redis" {
		t.Errorf("expected %q, got %q", "redis", svcs[1].Service)
	}
}

func TestDecodePS_JSONArray(t *testing.T) {
	out := "[" + postgresRunningPS + "]"

	svcs, err := decodePS(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Health != "healthy" {
		t.Errorf("expected %q, got %q", "healthy", svcs[0].Health)
	}
}

func TestDecodePS_Empty(t *testing.T) {
	svcs, err := decodePS("   \n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svcs) != 0 {
		t.Errorf("expected no services, got %d", len(svcs))
	}
}

func TestDecodePS_Malformed(t *testing.T) {
	_, err := decodePS("{not json}")
	if err == nil {
		t.Error("expected an error for malformed ps output")
	}
}

func TestDetected_ComposeYAML(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"compose.yaml": ""})

	got := Detected(root)
	if !got {
		t.Error("expected compose.yaml to be detected")
	}
}

func TestDetected_ComposeYML(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"compose.yml": ""})

	got := Detected(root)
	if !got {
		t.Error("expected compose.yml to be detected")
	}
}

func TestDetected_DockerComposeYAML(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"docker-compose.yaml": ""})

	got := Detected(root)
	if !got {
		t.Error("expected docker-compose.yaml to be detected")
	}
}

func TestDetected_DockerComposeYML(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"docker-compose.yml": ""})

	got := Detected(root)
	if !got {
		t.Error("expected docker-compose.yml to be detected")
	}
}

func TestDetected_FalseWhenAbsent(t *testing.T) {
	got := Detected(t.TempDir())
	if got {
		t.Error("expected no detection in an empty dir")
	}
}

func TestDetected_FalseWhenNameIsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docker-compose.yml"), 0755); err != nil {
		t.Fatal(err)
	}

	got := Detected(root)
	if got {
		t.Error("expected a directory named like a compose file not to count")
	}
}

func TestFindComposeFile_PrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	makeFiles(t, root, map[string]string{"compose.yaml": "", "docker-compose.yml": ""})

	name, ok := findComposeFile(root)
	if !ok {
		t.Fatal("expected a compose file to be found")
	}
	if name != "compose.yaml" {
		t.Errorf("expected %q to win precedence, got %q", "compose.yaml", name)
	}
}
