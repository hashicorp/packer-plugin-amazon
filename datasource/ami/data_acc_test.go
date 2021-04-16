package ami

import (
	_ "embed"
	"fmt"
	"os/exec"
	"testing"
	"time"

	amazon_acc "github.com/hashicorp/packer-plugin-amazon/builder/ebs/acceptance"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

//go:embed test-fixtures/template.pkr.hcl
var testDatasourceBasic string

func TestAccDatasource_AmazonAmi(t *testing.T) {
	ami := amazon_acc.AMIHelper{
		Region: "us-west-2",
		Name:   fmt.Sprintf("packer-amazon-ami-test %d", time.Now().Unix()),
	}
	testCase := &acctest.PluginTestCase{
		Name: "amazon_ami_datasource_basic_test",
		Teardown: func() error {
			return ami.CleanUpAmi()
		},
		Template: fmt.Sprintf(testDatasourceBasic, ami.Name),
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
