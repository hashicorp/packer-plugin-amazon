package ami

import (
	_ "embed"
	"fmt"
	"os/exec"
	"testing"

	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

//go:embed test-fixtures/template.pkr.hcl
var testDatasourceBasic string

func TestAccAmazonAmi(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "amazon_ami_datasource_basic_test",
		Teardown: func() error {
			helper := ebs.AMIHelper{
				Region: "us-west-2",
				Name:   "packer-amazon-ami-test",
			}
			return helper.CleanUpAmi()
		},
		Template: testDatasourceBasic,
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("Bad exit code. Logfile: %s", logfile)
				}
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
