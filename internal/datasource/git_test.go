package datasource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	pb "github.com/hashicorp/waypoint/internal/server/gen"
)

func TestGitProjectSource(t *testing.T) {
	cases := []struct {
		Name     string
		Input    string
		Expected *pb.Job_Git
	}{
		{
			"minimum",
			`
url = "foo"
`,
			&pb.Job_Git{
				Url: "foo",
			},
		},

		{
			"basic auth",
			`
url = "foo"
username = "alice"
password = "giraffe"
`,
			&pb.Job_Git{
				Url: "foo",
				Auth: &pb.Job_Git_Basic_{
					Basic: &pb.Job_Git_Basic{
						Username: "alice",
						Password: "giraffe",
					},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			// Parse the input
			f, diag := hclsyntax.ParseConfig([]byte(tt.Input), "<test>", hcl.Pos{})
			require.False(diag.HasErrors())

			// Get the project source value
			var s GitSource
			result, err := s.ProjectSource(f.Body, &hcl.EvalContext{})
			require.NoError(err)
			actual := result.Source.(*pb.Job_DataSource_Git).Git
			require.Equal(actual, tt.Expected)
		})
	}
}

func TestGitSourceOverride(t *testing.T) {
	cases := []struct {
		Name     string
		Input    *pb.Job_DataSource
		M        map[string]string
		Expected *pb.Job_DataSource
		Error    string
	}{
		{
			"nothing",
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: "foo",
					},
				},
			},
			nil,
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: "foo",
					},
				},
			},
			"",
		},

		{
			"ref",
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: "foo",
					},
				},
			},
			map[string]string{"ref": "bar"},
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: "foo",
						Ref: "bar",
					},
				},
			},
			"",
		},

		{
			"invalid",
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: "foo",
					},
				},
			},
			map[string]string{"other": "bar"},
			nil,
			"other",
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			var s GitSource
			err := s.Override(tt.Input, tt.M)
			if tt.Error != "" {
				require.Error(err)
				require.Contains(err.Error(), tt.Error)
				return
			}

			require.NoError(err)
			require.Equal(tt.Expected, tt.Input)
		})
	}
}

func TestGitSourceGet(t *testing.T) {
	t.Run("basic clone", func(t *testing.T) {
		require := require.New(t)

		var s GitSource
		dir, closer, err := s.Get(
			context.Background(),
			hclog.L(),
			terminal.ConsoleUI(context.Background()),
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: testGitFixture(t, "git-noop"),
					},
				},
			},
			"",
		)
		require.NoError(err)
		if closer != nil {
			defer closer()
		}

		// Verify files
		_, err = os.Stat(filepath.Join(dir, "waypoint.hcl"))
		require.NoError(err)
	})

	t.Run("branch ref", func(t *testing.T) {
		require := require.New(t)

		var s GitSource
		dir, closer, err := s.Get(
			context.Background(),
			hclog.L(),
			terminal.ConsoleUI(context.Background()),
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: testGitFixture(t, "git-refs"),
						Ref: "branch",
					},
				},
			},
			"",
		)
		require.NoError(err)
		if closer != nil {
			defer closer()
		}

		// Verify files
		_, err = os.Stat(filepath.Join(dir, "waypoint.hcl"))
		require.NoError(err)
		_, err = os.Stat(filepath.Join(dir, "branchfile"))
		require.NoError(err)
	})

	t.Run("commit", func(t *testing.T) {
		require := require.New(t)

		var s GitSource
		dir, closer, err := s.Get(
			context.Background(),
			hclog.L(),
			terminal.ConsoleUI(context.Background()),
			&pb.Job_DataSource{
				Source: &pb.Job_DataSource_Git{
					Git: &pb.Job_Git{
						Url: testGitFixture(t, "git-refs"),
						Ref: "29758b9",
					},
				},
			},
			"",
		)
		require.NoError(err)
		if closer != nil {
			defer closer()
		}

		// Verify files
		_, err = os.Stat(filepath.Join(dir, "waypoint.hcl"))
		require.NoError(err)
		_, err = os.Stat(filepath.Join(dir, "two"))
		require.Error(err)
	})
}

// testGitFixture MUST be called before TestRunner since TestRunner
// changes our working directory.
func testGitFixture(t *testing.T, n string) string {
	t.Helper()

	// Get our full path
	wd, err := os.Getwd()
	require.NoError(t, err)
	wd, err = filepath.Abs(wd)
	require.NoError(t, err)
	path := filepath.Join(wd, "testdata", n)

	// Look for a DOTgit
	original := filepath.Join(path, "DOTgit")
	_, err = os.Stat(original)
	require.NoError(t, err)

	// Rename it
	newPath := filepath.Join(path, ".git")
	require.NoError(t, os.Rename(original, newPath))
	t.Cleanup(func() { os.Rename(newPath, original) })

	return path
}
