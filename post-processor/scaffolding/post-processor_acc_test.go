package scaffolding

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/scaffolding/post-processor_acc_test.go  -timeout=120m
func TestScaffoldingPostProcessor(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "scaffolding_post-processor_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testPostProcessorHCL2Basic,
		Type:     "scaffolding-my-post-processor",
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}

			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := ioutil.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			postProcessorOutputLog := "post-processor mock: my-mock-config"
			if matched, _ := regexp.MatchString(postProcessorOutputLog+".*", logsString); !matched {
				t.Fatalf("logs doesn't contain expected foo value %q", logsString)
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

const testPostProcessorHCL2Basic = `
source "null" "basic-example" {
  communicator = "none"
}

build {
  sources = [
    "source.null.basic-example"
  ]

  post-processor "scaffolding-my-post-processor" {
    mock = "my-mock-config"
  }
}
`
