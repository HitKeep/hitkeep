package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/devtool"
	json "hitkeep/jsonapi"
)

func prepareCLIAction(t *testing.T) (string, *devtool.App) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.CommandContext(t.Context(), "go", "build", "-race", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	dashboard := filepath.Join(workspace, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":                               "module example.test/hk\n\ngo 1.27.1\n",
		"CONTRIBUTING.md":                      "# Contributing\n",
		"frontend/dashboard/package.json":      "{\"packageManager\":\"npm@12.1.0\"}\n",
		"frontend/dashboard/package-lock.json": "{}\n",
		"frontend/dashboard/.npmrc":            "legacy-peer-deps=true\n",
		"frontend/dashboard/.node-version":     "26.10.0\n",
	} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"docker": "#!/bin/sh\necho 1.0.0\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stateDir := filepath.Join(t.TempDir(), "state")
	installFakeManagedToolchain(t, stateDir)
	t.Setenv("HK_STATE_DIR", stateDir)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app, err := devtool.NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return binary, app
}

func TestDetachedCLIActionOutlivesLauncher(t *testing.T) {
	binary, app := prepareCLIAction(t)
	awaitEntry, release := blockCLIAction(t)
	cleanup := registerRunCleanup(t, app)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--workspace", app.Root(), "--output", "json", "setup", "--detach")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("start detached setup: %v: %s", err, output)
	}
	awaitEntry()
	var envelope struct {
		Data devtool.RunStart `json:"data"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("decode detached setup: %v: %s", err, output)
	}
	if envelope.Data.RunID == "" {
		t.Fatalf("detached setup returned no run ID: %s", output)
	}
	run, err := app.GetRun(envelope.Data.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "running" || run.PID <= 0 || command.ProcessState == nil || !command.ProcessState.Exited() {
		t.Fatalf("launcher did not exit independently of blocked action: %+v", run)
	}
	release()
	run, err = app.WaitRunObserved(ctx, run.ID, nil)
	if err != nil || run.Status != "passed" {
		t.Fatalf("setup completion: %+v, %v", run, err)
	}
	cleanup()
}

// Registration precedes launch and never depends on decoding a response's run ID.
func registerRunCleanup(t *testing.T, app *devtool.App) func() {
	t.Helper()
	cleanup := sync.OnceFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		runs, err := app.ListRuns(100)
		if err != nil {
			t.Errorf("list owned runs during cleanup: %v", err)
			return
		}
		for _, run := range runs {
			if run.Status == "passed" || run.Status == "failed" || run.Status == "cancelled" {
				continue
			}
			if _, err := app.CancelRun(run.ID); err != nil {
				t.Errorf("cancel owned run %s: %v", run.ID, err)
			}
			final, err := app.WaitRunObserved(ctx, run.ID, nil)
			if final.Status != "passed" && final.Status != "failed" && final.Status != "cancelled" {
				t.Errorf("owned writer did not quiesce: %+v, %v", final, err)
			}
		}
	})
	t.Cleanup(cleanup)
	return cleanup
}

func blockCLIAction(t *testing.T) (func(), func()) {
	t.Helper()
	directory := t.TempDir()
	entryPath, releasePath := filepath.Join(directory, "entered"), filepath.Join(directory, "release")
	var pipes []*os.File
	for _, path := range []string{entryPath, releasePath} {
		if err := syscall.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
		pipe, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		pipes = append(pipes, pipe)
		t.Cleanup(func() { _ = pipe.Close() })
	}
	npx := filepath.Join(os.Getenv("HK_STATE_DIR"), "shared", "toolchains", "node-26.10.0-"+runtime.GOOS+"-"+runtime.GOARCH, "bin", "npx")
	script := fmt.Sprintf("#!/bin/sh\nprintf x > %q\nread release < %q\necho action-released\n", entryPath, releasePath)
	if err := os.WriteFile(npx, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	entered := make(chan error, 1)
	go func() { var b [1]byte; _, err := pipes[0].Read(b[:]); entered <- err }()
	release := sync.OnceFunc(func() {
		if _, err := pipes[1].Write([]byte("release\n")); err != nil {
			t.Errorf("release action: %v", err)
		}
	})
	t.Cleanup(release)
	return func() {
		t.Helper()
		select {
		case err := <-entered:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("setup did not enter blocked action")
		}
	}, release
}

func TestDetachedCLIActionCleanupWithoutDecodedRunID(t *testing.T) {
	binary, app := prepareCLIAction(t)
	awaitEntry, _ := blockCLIAction(t)
	cleanup := registerRunCleanup(t, app)
	command := exec.CommandContext(t.Context(), binary, "--workspace", app.Root(), "--output", "json", "setup", "--detach")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("launch: %v: %s", err, output)
	}
	awaitEntry()
	for _, output := range []string{"malformed", `{"data":{}}`} {
		var envelope struct {
			Data devtool.RunStart `json:"data"`
		}
		err := json.Unmarshal([]byte(output), &envelope)
		if err == nil && envelope.Data.RunID != "" {
			t.Fatal("fixture unexpectedly supplied a run ID")
		}
	}
	cleanup()
	runs, err := app.ListRuns(100)
	if err != nil || len(runs) != 1 || runs[0].Status != "cancelled" {
		t.Fatalf("undecoded run cleanup: %+v, %v", runs, err)
	}
}

func TestMCPStdioWritesNoNonProtocolOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	root := repositoryRoot(t)
	command := exec.Command("go", "run", "./cmd/hk", "mcp", "serve", "--workspace", root)
	command.Dir = root
	command.Stdin = bytes.NewReader(nil)
	command.Env = append(os.Environ(), "HK_STATE_DIR="+filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("serve with closed stdin: %v: %s", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("non-protocol stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestMCPStdioWithoutWorkspaceUsesConfiguredFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workspace, "CONTRIBUTING.md"), []byte("# Contributing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module hitkeep\n\ngo 1.27.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	delegated := filepath.Join(t.TempDir(), "delegated")
	launcher := "#!/bin/sh\n: > " + delegated + "\nexec " + binary + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(workspace, "hk"), []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateDir)
	wantApp, err := devtool.NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.Command(binary, "mcp", "serve", "--fallback-workspace", workspace)
	command.Dir = projectRoot
	command.Env = append(os.Environ(), "HK_STATE_DIR="+stateDir)
	client := mcp.NewClient(&mcp.Implementation{Name: "hk-central-integration-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	envelope := result.StructuredContent.(map[string]any)
	if result.IsError || envelope["workspace_id"] != wantApp.WorkspaceID() {
		t.Fatalf("central stdio server did not route by client root: %#v", envelope)
	}
	if _, err := os.Stat(delegated); !os.IsNotExist(err) {
		t.Fatalf("central server unexpectedly launched a nested workspace MCP: %v", err)
	}
}

func TestMCPStdioActionRunLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	dashboard := filepath.Join(workspace, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":                               "module example.test/hk\n\ngo 1.27.1\n",
		"CONTRIBUTING.md":                      "# Contributing\n",
		"frontend/dashboard/package.json":      "{\"packageManager\":\"npm@12.1.0\"}\n",
		"frontend/dashboard/package-lock.json": "{}\n",
		"frontend/dashboard/.npmrc":            "legacy-peer-deps=true\n",
		"frontend/dashboard/.node-version":     "26.10.0\n",
	} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	slowFile := filepath.Join(t.TempDir(), "slow")
	goScript := "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'go version go1.27.1 test'; exit 0; fi\nif [ -f \"" + slowFile + "\" ]; then sleep 30; fi\nexit 0\n"
	dockerScript := "#!/bin/sh\ncase \"$*\" in *version*) echo 1.0.0; exit 0;; esac\nif [ -f \"" + slowFile + "\" ]; then sleep 30; fi\nexit 0\n"
	for name, script := range map[string]string{
		"docker": dockerScript, "go": goScript, "npm": "#!/bin/sh\nif [ \"${1:-}\" = --version ]; then echo '12.1.0'; exit 0; fi\nmkdir -p node_modules\nexit 0\n", "node": "#!/bin/sh\necho v26.10.0\n", "npx": "#!/bin/sh\nmkdir -p node_modules\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stateDir := filepath.Join(t.TempDir(), "state")
	installFakeManagedToolchain(t, stateDir)
	command := exec.Command(binary, "mcp", "serve", "--workspace", workspace)
	command.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "HK_STATE_DIR="+stateDir)
	client := mcp.NewClient(&mcp.Implementation{Name: "hk-integration-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	t.Setenv("HK_STATE_DIR", stateDir)
	app, err := devtool.NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	registerRunCleanup(t, app)
	started, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_start", Arguments: map[string]any{"kind": "setup"}})
	if err != nil {
		t.Fatal(err)
	}
	if started.IsError {
		t.Fatalf("run start failed: %s", started.Content[0].(*mcp.TextContent).Text)
	}
	envelope := started.StructuredContent.(map[string]any)
	data := envelope["data"].(map[string]any)
	runID := data["run_id"].(string)
	for {
		statusResult, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": runID}})
		if callErr != nil {
			t.Fatal(callErr)
		}
		statusEnvelope := statusResult.StructuredContent.(map[string]any)
		run := statusEnvelope["data"].(map[string]any)
		status := run["status"].(string)
		if status == "passed" {
			break
		}
		if status == "failed" || status == "cancelled" {
			t.Fatalf("setup run ended as %s: %#v", status, run)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%v: last run state %#v", ctx.Err(), run)
		case <-time.After(50 * time.Millisecond):
		}
	}

	if err := os.WriteFile(slowFile, []byte("slow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_start", Arguments: map[string]any{"kind": "setup"}})
	if err != nil {
		t.Fatal(err)
	}
	envelope = started.StructuredContent.(map[string]any)
	data = envelope["data"].(map[string]any)
	runID = data["run_id"].(string)
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_cancel", Arguments: map[string]any{"run_id": runID}}); err != nil {
		t.Fatal(err)
	}
	for {
		statusResult, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": runID}})
		if callErr != nil {
			t.Fatal(callErr)
		}
		statusEnvelope := statusResult.StructuredContent.(map[string]any)
		run := statusEnvelope["data"].(map[string]any)
		status := run["status"].(string)
		if status == "cancelled" {
			break
		}
		if status == "failed" || status == "passed" {
			t.Fatalf("cancelled setup ended as %s: %#v", status, run)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%v: last cancelled run state %#v", ctx.Err(), run)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func installFakeManagedToolchain(t *testing.T, stateDir string) {
	t.Helper()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	goBin := filepath.Join(stateDir, "shared", "toolchains", "go-1.27.1-"+platform, "bin")
	nodeRoot := filepath.Join(stateDir, "shared", "toolchains", "node-26.10.0-"+platform)
	nodeBin := filepath.Join(nodeRoot, "bin")
	npmCLI := filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	for _, directory := range []string{goBin, nodeBin, filepath.Dir(npmCLI)} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	managedCommands := map[string]string{
		filepath.Join(goBin, "go"):     "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'go version go1.27.1 test'; fi\nexit 0\n",
		filepath.Join(nodeBin, "node"): "#!/bin/sh\ncase \"${1:-}\" in *npm-cli.js) echo 12.1.0 ;; *) echo v26.10.0 ;; esac\nexit 0\n",
		filepath.Join(nodeBin, "npm"):  "#!/bin/sh\nif [ \"${1:-}\" = --version ]; then echo '12.1.0'; exit 0; fi\nmkdir -p node_modules\nexit 0\n",
		filepath.Join(nodeBin, "npx"):  "#!/bin/sh\nmkdir -p node_modules\nexit 0\n",
		npmCLI:                         "#!/bin/sh\nexit 0\n",
	}
	for path, script := range managedCommands {
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunWorkerExitsBeforeStateRemoval(t *testing.T) {
	binary, app := prepareCLIAction(t)
	workspace, err := app.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	run := devtool.Run{ID: "direct-child", WorkspaceID: workspace.ID, Request: devtool.RunRequest{Kind: "setup"}, Status: "queued", StartedAt: time.Now().UTC(), LogPath: filepath.Join(workspace.StateDir, "runs", "direct-child.log")}
	if err := os.MkdirAll(filepath.Join(workspace.StateDir, "runs", "tmp", run.ID), 0700); err != nil {
		t.Fatal(err)
	}
	writeRecord := func() {
		t.Helper()
		raw, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace.StateDir, "runs", run.ID+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeRecord()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, binary, "__run", "--workspace", workspace.Root, "--run-id", run.ID)
	command.Env = append(os.Environ(), "HK_CHILD_RUN=1", "HK_EXPECTED_SCHEMA="+devtool.SchemaVersion, "HK_WORKER_PROTOCOL=1", "HK_WORKSPACE_ID="+workspace.ID, "HK_SOURCE_FINGERPRINT=legacy")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	wait := sync.OnceValue(command.Wait)
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_, _ = app.CancelRun(run.ID)
		}
		if err := wait(); err != nil {
			t.Errorf("worker exit: %v: %s", err, output.String())
		}
	})
	run.PID = command.Process.Pid
	run.Status = "running"
	writeRecord()
	if err := os.WriteFile(filepath.Join(workspace.StateDir, "runs", run.ID+".ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	terminal, err := app.WaitRunObserved(ctx, run.ID, nil)
	if err != nil {
		t.Fatalf("worker completion: %+v, %v", terminal, err)
	}
	retained := make(map[string][]byte)
	for _, path := range []string{run.LogPath, filepath.Join(workspace.StateDir, "runs", run.ID+".json"), filepath.Join(workspace.StateDir, "artifacts", "index.json")} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		retained[path] = raw
	}
	if err := wait(); err != nil {
		t.Fatalf("worker Wait: %v: %s", err, output.String())
	}
	if !command.ProcessState.Success() {
		t.Fatal("worker did not exit successfully")
	}
	for path, want := range retained {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("post-terminal write to %s: %v", path, err)
		}
	}
	for _, path := range []string{filepath.Join(workspace.StateDir, "runs", "tmp", run.ID), filepath.Join(workspace.StateDir, "runs", run.ID+".ready"), filepath.Join(workspace.StateDir, "runs", run.ID+".cancel")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("transient survives child exit: %s, %v", path, err)
		}
	}
	if err := os.RemoveAll(workspace.StateDir); err != nil {
		t.Fatalf("remove joined child's state: %v", err)
	}
}

func TestDetachedCLIActionCleanupAfterEarlyAssertion(t *testing.T) {
	const helper = "HK_TEST_EARLY_ASSERTION"
	if os.Getenv(helper) == "1" {
		binary, app := prepareCLIAction(t)
		awaitEntry, _ := blockCLIAction(t)
		t.Cleanup(func() {
			runs, err := app.ListRuns(100)
			if err != nil || len(runs) != 1 || runs[0].Status != "cancelled" {
				t.Errorf("early assertion left active writer: %+v, %v", runs, err)
				return
			}
			t.Log("early assertion writer quiesced")
		})
		registerRunCleanup(t, app)
		command := exec.CommandContext(t.Context(), binary, "--workspace", app.Root(), "--output", "json", "setup", "--detach")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("launch: %v: %s", err, output)
		}
		awaitEntry()
		t.Fatal("intentional assertion before decoding run ID")
	}
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDetachedCLIActionCleanupAfterEarlyAssertion$", "-test.v")
	command.Env = append(os.Environ(), helper+"=1")
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("intentional assertion before decoding run ID")) || !bytes.Contains(output, []byte("early assertion writer quiesced")) || bytes.Contains(output, []byte("TempDir RemoveAll cleanup")) {
		t.Fatalf("early assertion cleanup proof: %v: %s", err, output)
	}
}
