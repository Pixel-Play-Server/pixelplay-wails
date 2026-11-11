package horizon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func updateFixture(t *testing.T, dir, version string) Runtime {
	t.Helper()
	r := at(filepath.Join(dir, ".horizon-native", version))
	if err := os.MkdirAll(r.Resources, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{r.Binary, filepath.Join(r.Resources, "horizon-resources.json")} {
		if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return r
}
func TestUpdateKeepsWorkingRuntimeOnFailure(t *testing.T) {
	dir := t.TempDir()
	old := updateFixture(t, dir, Version)
	now := time.Now()
	ops := updateOperations{now: func() time.Time { return now }, discover: func(context.Context, string) (runtimeRelease, error) { return runtimeRelease{}, errors.New("offline") }, install: func(context.Context, string, runtimeRelease, func(string)) (Runtime, error) {
		t.Fatal("offline update installed")
		return Runtime{}, nil
	}}
	got, err := ensureUpdated(context.Background(), dir, nil, ops)
	if err != nil || got.Root != old.Root || Status(dir).LastError != "offline" {
		t.Fatalf("lost fallback: %v %v", got, err)
	}
	ops.discover = func(context.Context, string) (runtimeRelease, error) {
		return runtimeRelease{Version: "nightly-" + strings.Repeat("a", 64), Digest: strings.Repeat("a", 64)}, nil
	}
	ops.install = func(context.Context, string, runtimeRelease, func(string)) (Runtime, error) {
		return Runtime{}, errors.New("bad checksum")
	}
	got, err = ensureUpdated(context.Background(), dir, nil, ops)
	if err != nil || got.Root != old.Root {
		t.Fatalf("failed install replaced active: %v", err)
	}
}
func TestUpdateCommitsReadyRuntimeAndCaches(t *testing.T) {
	dir := t.TempDir()
	updateFixture(t, dir, Version)
	digest := strings.Repeat("b", 64)
	version := "nightly-" + digest
	calls := 0
	ops := updateOperations{now: time.Now, discover: func(context.Context, string) (runtimeRelease, error) {
		calls++
		return runtimeRelease{Version: version, Digest: digest}, nil
	}, install: func(context.Context, string, runtimeRelease, func(string)) (Runtime, error) {
		return updateFixture(t, dir, version), nil
	}}
	got, err := ensureUpdated(context.Background(), dir, nil, ops)
	if err != nil {
		t.Fatal(err)
	}
	if readUpdateState(dir).Active != version || filepath.Base(got.Root) != version {
		t.Fatal("new runtime not committed")
	}
	if _, err = ensureUpdated(context.Background(), dir, nil, ops); err != nil || calls != 1 {
		t.Fatal("cache ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ensureUpdated(ctx, dir, nil, ops); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	if !file(filepath.Join(dir, ".horizon-native", Version, exe("horizon-client"))) {
		t.Fatal("old runtime deleted")
	}
}
func TestReleaseRejectsUnverifiedSources(t *testing.T) {
	asset := "horizon-linux-x86_64.tar.gz"
	digest := strings.Repeat("c", 64)
	raw := `{"tag_name":"nightly","assets":[{"name":"` + asset + `","size":10,"digest":"sha256:` + digest + `","browser_download_url":"` + releaseBase + `nightly/` + asset + `"}]}`
	if _, err := decodeRelease(strings.NewReader(raw), asset); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(raw, "github.com", "example.com", 1), strings.Replace(raw, "sha256:", "md5:", 1), strings.Replace(raw, `"size":10`, `"size":0`, 1)} {
		if _, err := decodeRelease(strings.NewReader(bad), asset); err == nil {
			t.Fatal("unverified release accepted")
		}
	}
}
func TestCompatibilityRefusesChangedMinecraft(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "research-lock.json")
	os.WriteFile(p, []byte(`{"minecraft":{"version":"99.0","java_major":25}}`), 0600)
	if checkRuntimeCompatibility(dir) == nil {
		t.Fatal("incompatible engine accepted")
	}
	os.WriteFile(p, []byte(`{"minecraft":{"version":"26.3","java_major":25}}`), 0600)
	if err := checkRuntimeCompatibility(dir); err != nil {
		t.Fatal(err)
	}
}
