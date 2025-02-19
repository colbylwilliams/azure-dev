package kubectl

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/test/mocks"
	"github.com/azure/azure-dev/cli/azd/test/ostest"
	"github.com/stretchr/testify/require"
)

func Test_ApplyFiles(t *testing.T) {
	tempDir := t.TempDir()
	ostest.Chdir(t, tempDir)

	ran := false
	var runArgs exec.RunArgs

	mockContext := mocks.NewMockContext(context.Background())
	mockContext.CommandRunner.When(func(args exec.RunArgs, command string) bool {
		return strings.Contains(command, "kubectl apply -f")
	}).RespondFn(func(args exec.RunArgs) (exec.RunResult, error) {
		runArgs = args
		ran = true

		return exec.NewRunResult(0, "", ""), nil
	})

	cli := NewKubectl(mockContext.CommandRunner)

	err := os.WriteFile("test.yaml", []byte("yaml"), osutil.PermissionFile)
	require.NoError(t, err)

	err = cli.Apply(*mockContext.Context, tempDir, &KubeCliFlags{
		Namespace: "test-namespace",
	})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(runArgs.StdIn)
	require.NoError(t, err)

	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, "kubectl", runArgs.Cmd)
	require.Equal(t, "yaml", buf.String())
	require.Equal(t, []string{"apply", "-f", "-", "-n", "test-namespace"}, runArgs.Args)
}

func Test_Command_Args(t *testing.T) {
	tempDir := t.TempDir()
	ostest.Chdir(t, tempDir)

	mockContext := mocks.NewMockContext(context.Background())
	cli := NewKubectl(mockContext.CommandRunner)

	tests := map[string]*kubeCliTestConfig{
		"apply-with-input": {
			mockCommandPredicate: "kubectl apply -f -",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"apply", "-f", "-", "-n", "test-namespace"},
			testFn: func() error {
				_, err := cli.ApplyWithInput(*mockContext.Context, "input", &KubeCliFlags{
					Namespace: "test-namespace",
				})

				return err
			},
		},
		"config-view": {
			mockCommandPredicate: "kubectl config view",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"config", "view", "--merge", "--flatten"},
			testFn: func() error {
				_, err := cli.ConfigView(*mockContext.Context, true, true, nil)

				return err
			},
		},
		"config-use-context": {
			mockCommandPredicate: "kubectl config use-context",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"config", "use-context", "context-name"},
			testFn: func() error {
				_, err := cli.ConfigUseContext(*mockContext.Context, "context-name", nil)

				return err
			},
		},
		"create-namespace": {
			mockCommandPredicate: "kubectl create namespace",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"create", "namespace", "namespace-name", "--dry-run=client", "-o", "yaml"},
			testFn: func() error {
				_, err := cli.CreateNamespace(*mockContext.Context, "namespace-name", &KubeCliFlags{
					DryRun: DryRunTypeClient,
					Output: OutputTypeYaml,
				})

				return err
			},
		},
		"create-secret": {
			mockCommandPredicate: "kubectl create secret generic",
			expectedCmd:          "kubectl",
			expectedArgs: []string{
				"create",
				"secret",
				"generic",
				"secret-name",
				"--from-literal=foo=bar",
				"-n",
				"test-namespace",
			},
			testFn: func() error {
				_, err := cli.CreateSecretGenericFromLiterals(
					*mockContext.Context,
					"secret-name",
					[]string{"foo=bar"},
					&KubeCliFlags{
						Namespace: "test-namespace",
					},
				)

				return err
			},
		},
		"rollout-status": {
			mockCommandPredicate: "kubectl rollout status",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"rollout", "status", "deployment/deployment-name", "-n", "test-namespace"},
			testFn: func() error {
				_, err := cli.RolloutStatus(*mockContext.Context, "deployment-name", &KubeCliFlags{
					Namespace: "test-namespace",
				})

				return err
			},
		},
		"exec": {
			mockCommandPredicate: "kubectl get deployment",
			expectedCmd:          "kubectl",
			expectedArgs:         []string{"get", "deployment", "-n", "test-namespace", "-o", "json"},
			testFn: func() error {
				_, err := cli.Exec(*mockContext.Context, &KubeCliFlags{
					Namespace: "test-namespace",
					Output:    OutputTypeJson,
				}, "get", "deployment")

				return err
			},
		},
	}

	for testName, config := range tests {
		mockContext.CommandRunner.When(func(args exec.RunArgs, command string) bool {
			return strings.Contains(command, config.mockCommandPredicate)
		}).RespondFn(func(args exec.RunArgs) (exec.RunResult, error) {
			config.ran = true
			config.actualArgs = &args

			return exec.NewRunResult(0, config.mockCommandResult, ""), nil
		})

		t.Run(testName, func(t *testing.T) {
			err := config.testFn()
			require.NoError(t, err)
			require.True(t, config.ran)
			require.Equal(t, config.expectedCmd, config.actualArgs.Cmd)
			require.Equal(t, config.expectedArgs, config.actualArgs.Args)
		})
	}
}

type kubeCliTestConfig struct {
	mockCommandPredicate string
	mockCommandResult    string
	expectedCmd          string
	expectedArgs         []string
	actualArgs           *exec.RunArgs
	ran                  bool
	testFn               func() error
}
