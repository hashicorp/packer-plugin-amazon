package amazon_acc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func GenerateSSHPrivateKeyFile() (string, error) {
	outFile := fmt.Sprintf("%s/temp_key.ed25519", os.TempDir())
	sshGenCmd := exec.Command("ssh-keygen", "-t", "ed25519", "-b", "256", "-f", outFile)
	sshGenCmd.Stdin = bytes.NewBuffer([]byte("\n\n"))

	_, err := sshGenCmd.CombinedOutput()
	if err != nil {
		os.Remove(outFile)
		return "", err
	}

	return outFile, nil
}
