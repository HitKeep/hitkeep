package devtool

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestReleasePublicationJobsAreIndependent(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/release-publish.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Needs any
		}
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"github", "npm", "images", "helm"} {
		job, ok := workflow.Jobs[channel]
		if !ok || job.Needs != "verify" {
			t.Errorf("%s must depend only on verified candidate evidence", channel)
		}
	}
}
