package cops

import (
	"os"
	"reflect"
	"testing"
)

func TestBuildWithLoginUserShell(t *testing.T) {
	userToShell["nitrous"] = "/bin/false"

	eCmd := buildWithLoginUserShell("apt-get install php5")

	expectedArgs := []string{
		"/bin/su",
		"--login",
		"-c",
		"/bin/false -i -l -c 'apt-get install php5'",
		"nitrous",
	}

	if !reflect.DeepEqual(eCmd.Args, expectedArgs) {
		t.Errorf("unexpected cmd args %v", expectedArgs, eCmd.Args)
	}

	os.Setenv("NITROUS_VERSION", "1")
	os.Setenv("NITROUS_ENV", "VAR")

	eCmd = buildWithLoginUserShell("apt-get install php5")

	expectedArgs[3] = "/bin/false -i -l -c 'NITROUS_VERSION=1 NITROUS_ENV=VAR apt-get install php5'"
	if !reflect.DeepEqual(eCmd.Args, expectedArgs) {
		t.Errorf("Expected %v, got %v", expectedArgs, eCmd.Args)
	}
}
