// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/blang/semver/v4"
)

type GitCli interface {
	tools.ExternalTool
	GetRemoteUrl(ctx context.Context, string, remoteName string) (string, error)
	ShallowClone(ctx context.Context, repositoryPath string, branch string, target string) error
	InitRepo(ctx context.Context, repositoryPath string) error
	AddRemote(ctx context.Context, repositoryPath string, remoteName string, remoteUrl string) error
	UpdateRemote(ctx context.Context, repositoryPath string, remoteName string, remoteUrl string) error
	GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error)
	AddFile(ctx context.Context, repositoryPath string, filespec string) error
	Commit(ctx context.Context, repositoryPath string, message string) error
	PushUpstream(ctx context.Context, repositoryPath string, origin string, branch string) error
	IsUntrackedFile(ctx context.Context, repositoryPath string, filePath string) (bool, error)
	SetCredentialStore(ctx context.Context, repositoryPath string) error
	ListStagedFiles(ctx context.Context, repositoryPath string) (string, error)
	AddFileExecPermission(ctx context.Context, repositoryPath string, file string) error
	// make current repo to use gh-cli as credential helper.
	SetGitHubAuthForRepo(ctx context.Context, repositoryPath, credential, ghPath string) error
}

type gitCli struct {
	commandRunner exec.CommandRunner
}

func NewGitCli(commandRunner exec.CommandRunner) GitCli {
	return &gitCli{
		commandRunner: commandRunner,
	}
}

func (cli *gitCli) versionInfo() tools.VersionInfo {
	return tools.VersionInfo{
		// Support version from 09-Dec-2018 08:40
		// https://mirrors.edge.kernel.org/pub/software/scm/git/
		// 4 years should cover most Linux out of the box version
		MinimumVersion: semver.Version{
			Major: 2,
			Minor: 20,
			Patch: 0},
		UpdateCommand: "Visit https://git-scm.com/downloads to upgrade",
	}
}

func (cli *gitCli) CheckInstalled(ctx context.Context) (bool, error) {
	found, err := tools.ToolInPath("git")
	if !found {
		return false, err
	}
	gitRes, err := tools.ExecuteCommand(ctx, cli.commandRunner, "git", "--version")
	if err != nil {
		return false, fmt.Errorf("checking %s version: %w", cli.Name(), err)
	}
	gitSemver, err := tools.ExtractVersion(gitRes)
	if err != nil {
		return false, fmt.Errorf("converting to semver version fails: %w", err)
	}
	updateDetail := cli.versionInfo()
	if gitSemver.LT(updateDetail.MinimumVersion) {
		return false, &tools.ErrSemver{ToolName: cli.Name(), VersionInfo: updateDetail}
	}
	return true, nil
}

func (cli *gitCli) InstallUrl() string {
	return "https://git-scm.com/downloads"
}

func (cli *gitCli) Name() string {
	return "git CLI"
}

func (cli *gitCli) ShallowClone(ctx context.Context, repositoryPath string, branch string, target string) error {
	args := []string{"clone", "--depth", "1", repositoryPath}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, target)

	runArgs := newRunArgs(args...)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to clone repository %s, %s: %w", repositoryPath, res.String(), err)
	}

	return nil
}

var noSuchRemoteRegex = regexp.MustCompile("(fatal|error): No such remote")
var notGitRepositoryRegex = regexp.MustCompile("(fatal|error): not a git repository")
var ErrNoSuchRemote = errors.New("no such remote")
var ErrNotRepository = errors.New("not a git repository")
var gitUntrackedFileRegex = regexp.MustCompile("untracked files present|new file")

func (cli *gitCli) GetRemoteUrl(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	runArgs := newRunArgs("-C", repositoryPath, "remote", "get-url", remoteName)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if noSuchRemoteRegex.MatchString(res.Stderr) {
		return "", ErrNoSuchRemote
	} else if notGitRepositoryRegex.MatchString(res.Stderr) {
		return "", ErrNotRepository
	} else if err != nil {
		return "", fmt.Errorf("failed to get remote url: %s: %w", res.String(), err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

func (cli *gitCli) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	runArgs := newRunArgs("-C", repositoryPath, "branch", "--show-current")
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if notGitRepositoryRegex.MatchString(res.Stderr) {
		return "", ErrNotRepository
	} else if err != nil {
		return "", fmt.Errorf("failed to get current branch: %s: %w", res.String(), err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

func (cli *gitCli) InitRepo(ctx context.Context, repositoryPath string) error {
	runArgs := newRunArgs("-C", repositoryPath, "init")
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to init repository: %s: %w", res.String(), err)
	}

	// Set initial branch to main
	runArgs = newRunArgs("-C", repositoryPath, "checkout", "-b", "main")
	res, err = cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to create main branch: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) SetCredentialStore(ctx context.Context, repositoryPath string) error {
	runArgs := newRunArgs("-C", repositoryPath, "config", "credential.helper", "store")
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to set credential store repository: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) AddRemote(ctx context.Context, repositoryPath string, remoteName string, remoteUrl string) error {
	runArgs := newRunArgs("-C", repositoryPath, "remote", "add", remoteName, remoteUrl)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to add remote: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) UpdateRemote(ctx context.Context, repositoryPath string, remoteName string, remoteUrl string) error {
	runArgs := newRunArgs("-C", repositoryPath, "remote", "set-url", remoteName, remoteUrl)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to add remote: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) AddFile(ctx context.Context, repositoryPath string, filespec string) error {
	runArgs := newRunArgs("-C", repositoryPath, "add", filespec)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to add files: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) Commit(ctx context.Context, repositoryPath string, message string) error {
	runArgs := newRunArgs("-C", repositoryPath, "commit", "--allow-empty", "-m", message)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to commit: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) PushUpstream(ctx context.Context, repositoryPath string, origin string, branch string) error {
	runArgs := newRunArgs("-C", repositoryPath, "push", "--set-upstream", "--quiet", origin, branch).
		WithInteractive(true)

	res, err := cli.commandRunner.Run(ctx, runArgs)

	if err != nil {
		return fmt.Errorf("failed to push: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) ListStagedFiles(ctx context.Context, repositoryPath string) (string, error) {
	runArgs := newRunArgs("-C", repositoryPath, "ls-files", "--stage")
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return "", fmt.Errorf("failed to list files: %s: %w", res.String(), err)
	}

	return res.Stdout, nil
}

func (cli *gitCli) AddFileExecPermission(ctx context.Context, repositoryPath string, file string) error {
	runArgs := newRunArgs("-C", repositoryPath, "update-index", "--add", "--chmod=+x", file)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return fmt.Errorf("failed to add file exec permission: %s: %w", res.String(), err)
	}

	return nil
}

func (cli *gitCli) IsUntrackedFile(ctx context.Context, repositoryPath string, filePath string) (bool, error) {
	runArgs := newRunArgs("-C", repositoryPath, "status", filePath)
	res, err := cli.commandRunner.Run(ctx, runArgs)
	if err != nil {
		return false, fmt.Errorf("failed to check status file: %s: %w", res.String(), err)
	}

	if gitUntrackedFileRegex.MatchString(res.Stdout) {
		return true, nil
	}

	return false, nil
}

// SetGitHubAuthForRepo creates git config for the repositoryPath like
//
// [credential "https://github.com"]  (when credential is equal to "https://github.com")
// helper =
// helper = !ghPath auth git-credential
//
// This way, git commands run from repositoryPath will use gh as auth credential.
// Note: Removes any previous configuration for the credential.
// Note: `helper = ` is intentional to break the chain of previously configured global helpers.
// See: https://github.com/cli/cli/issues/3796 for more about this strategy.
func (cli *gitCli) SetGitHubAuthForRepo(ctx context.Context, repositoryPath, credential, ghPath string) error {

	if err := setAuthCredentialHelper(
		ctx, cli.commandRunner, repositoryPath, credential, "", "replace-all"); err != nil {
		return err
	}
	// path needs to be quoted on windows
	if runtime.GOOS == "windows" {
		ghPath = fmt.Sprintf("'%s'", ghPath)
	}
	ghCredentialValue := fmt.Sprintf("!%s auth git-credential", ghPath)
	if err := setAuthCredentialHelper(
		ctx, cli.commandRunner, repositoryPath, credential, ghCredentialValue, "add"); err != nil {
		return err
	}

	return nil
}

func setAuthCredentialHelper(
	ctx context.Context, runner exec.CommandRunner, repositoryPath, credential, value, flag string) error {
	runArgs := newRunArgs(
		"-C", repositoryPath,
		"config", "--local", fmt.Sprintf("--%s", flag),
		fmt.Sprintf("credential.%s.helper", credential),
		value)

	if _, err := runner.Run(ctx, runArgs); err != nil {
		return fmt.Errorf("failed to set credential helper: %s='%s': %w", credential, value, err)
	}
	return nil
}

func newRunArgs(args ...string) exec.RunArgs {

	runArgs := exec.NewRunArgs("git", args...)
	if os.Getenv("CODESPACES") == "true" {
		// azd running git in codespaces should not use the Codespaces token.
		// As azd needs bigger access across repos. And the token in codespaces is mono-repo by default
		runArgs = runArgs.WithEnv([]string{"GITHUB_TOKEN=", "GH_TOKEN="})
	}

	return runArgs
}
