package e2e_test

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var serviceBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "e2e-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	binPath := filepath.Join(tmp, "notifier")
	cmd := exec.Command("go", "build", "-race", "-o", binPath, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("build failed: %s\n%s", err, out)
	}
	serviceBin = binPath

	os.Exit(m.Run())
}
