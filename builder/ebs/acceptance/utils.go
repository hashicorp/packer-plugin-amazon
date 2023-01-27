package amazon_acc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func GenerateSSHPrivateKeyFile() (string, error) {
	outFile := fmt.Sprintf("%s/temp_key", os.TempDir())

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", fmt.Errorf("failed to generate SSH key: %s", err)
	}

	x509key := x509.MarshalPKCS1PrivateKey(priv)

	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509key,
	})

	err = os.WriteFile(outFile, pemKey, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write private key to %q: %s", outFile, err)
	}

	return outFile, nil
}
