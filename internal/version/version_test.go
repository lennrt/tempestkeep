package version

import "testing"

func TestStringDevelopmentVersion(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = ""
	if got := String(); got != "v0.2.0-dev" {
		t.Fatalf("untagged development build = %q, want v0.2.0-dev", got)
	}
}

func TestStringBuildOverride(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "v9.8.7-test"
	if got := String(); got != version {
		t.Fatalf("build override = %q, want %q", got, version)
	}
}
