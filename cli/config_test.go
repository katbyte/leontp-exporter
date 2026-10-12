package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/leontp-metrics/lib/readings"
	"github.com/katbyte/leontp-metrics/sdk/leontp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// viper is global, so none of these can run in parallel and each resets it.
const (
	testServer   = "10.0.0.10"
	settingsFile = ".leontp-metrics"
)

// alone cuts a test off from the settings of whoever runs it: viper as it starts, an empty home and working directory, and none of the program's
// variables set.
func alone(t *testing.T) (home, wd string) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	home, wd = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(wd)
	t.Setenv(configEnv, "")
	for _, env := range envNames {
		t.Setenv(env, "")
	}

	return home, wd
}

// load drives the real flag wiring with these command-line arguments, after whatever the test put in the environment and the settings files.
func load(t *testing.T, args ...string) (*FlagData, error) {
	t.Helper()

	root := &cobra.Command{Use: toolName}
	if err := configureFlags(root); err != nil {
		t.Fatalf("configureFlags: %v", err)
	}
	if err := root.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := loadConfig(); err != nil {
		return nil, err
	}

	return GetFlags(), nil
}

// must is load for a test that expects the settings to be read.
func must(t *testing.T, args ...string) *FlagData {
	t.Helper()

	f, err := load(t, args...)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	return f
}

func write(t *testing.T, dir, file, body string) string {
	t.Helper()

	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// With nothing set the page is served where the README says and the time server is waited on as long as it says: these are what an example
// compose file and a scrape's own timeout are written against.
//
//nolint:paralleltest // viper is global state; these mutate it
func TestDefaults(t *testing.T) {
	alone(t)

	want := FlagData{Listen: ":9123", Timeout: time.Second}
	if got := *must(t); got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}
}

// Every setting has an environment variable, which is how a container is configured.
func TestEveryFlagHasAnEnvironmentVariable(t *testing.T) {
	alone(t)

	t.Setenv("LEONTP_METRICS_SERVER", testServer)
	t.Setenv("LEONTP_METRICS_LISTEN", "127.0.0.1:9000")
	t.Setenv("LEONTP_METRICS_LABELS", "hostname=ntp,job=leontp")
	t.Setenv("LEONTP_METRICS_TIMEOUT", "250ms")
	t.Setenv("LEONTP_METRICS_SHORT_QUESTION", "true")

	want := FlagData{Server: testServer, Listen: "127.0.0.1:9000", Labels: "hostname=ntp,job=leontp", Timeout: 250 * time.Millisecond, ShortQuestion: true}
	if got := *must(t); got != want {
		t.Errorf("from the environment = %+v, want %+v", got, want)
	}

	root := &cobra.Command{Use: toolName}
	if err := configureFlags(root); err != nil {
		t.Fatal(err)
	}
	root.PersistentFlags().VisitAll(func(fl *pflag.Flag) {
		if _, ok := envNames[fl.Name]; !ok && fl.Name != "config" {
			t.Errorf("--%s has no environment variable", fl.Name)
		}
	})
}

// A flag beats the environment, which beats a settings file: a container overrides what an image holds, and a command line overrides both.
func TestFlagThenEnvironmentThenFile(t *testing.T) {
	home, _ := alone(t)
	write(t, home, settingsFile, "SERVER=from-file\nLABELS=hostname=from-file\nTIMEOUT=3s\nSHORT_QUESTION=true\n")

	f := must(t)
	if f.Server != "from-file" || f.Labels != "hostname=from-file" || f.Timeout != 3*time.Second || !f.ShortQuestion {
		t.Errorf("from the file = %+v, want every setting it holds, the one of two words included", f)
	}

	viper.Reset()
	t.Setenv("LEONTP_METRICS_SERVER", "from-env")
	t.Setenv("LEONTP_METRICS_TIMEOUT", "2s")
	f = must(t)
	if f.Server != "from-env" || f.Timeout != 2*time.Second || f.Labels != "hostname=from-file" {
		t.Errorf("with the environment = %+v, want it over the file, and the file for the rest", f)
	}

	viper.Reset()
	f = must(t, "--server", "from-flag", "--timeout", "5s")
	if f.Server != "from-flag" || f.Timeout != 5*time.Second {
		t.Errorf("with flags = %+v, want them over the environment", f)
	}
}

// A .leontp-metrics in the working directory is found before the one in $HOME, and neither being there is not an error.
//
//nolint:paralleltest // viper is global state; these mutate it
func TestSettingsFilePrecedence(t *testing.T) {
	home, wd := alone(t)

	if f := must(t); f.Server != "" {
		t.Errorf("server = %q with no settings file, want none", f.Server)
	}

	viper.Reset()
	write(t, home, settingsFile, "SERVER=from-home\n")
	if f := must(t); f.Server != "from-home" {
		t.Errorf("server = %q, want the one in $HOME", f.Server)
	}

	viper.Reset()
	write(t, wd, settingsFile, "SERVER=from-here\n")
	if f := must(t); f.Server != "from-here" {
		t.Errorf("server = %q, want the working directory's over $HOME's", f.Server)
	}
}

// A settings file that is named is the only one read, and one that cannot be read is an error: a run told where its settings are must not come
// up with someone else's, or with none.
func TestANamedSettingsFile(t *testing.T) {
	home, _ := alone(t)
	write(t, home, settingsFile, "SERVER=from-home\n")
	named := write(t, home, "other.env", "LISTEN=:9000\n")

	f := must(t, "--config", named)
	if f.Listen != ":9000" || f.Server != "" {
		t.Errorf("with --config = %+v, want the named file and nothing of $HOME's", f)
	}

	viper.Reset()
	t.Setenv(configEnv, "~/other.env")
	if f := must(t); f.Listen != ":9000" {
		t.Errorf("with %s=~/other.env the page is served on %q, want the named file's", configEnv, f.Listen)
	}

	viper.Reset()
	if _, err := load(t, "--config", filepath.Join(home, "missing.env")); err == nil || !strings.Contains(err.Error(), "missing.env") {
		t.Errorf("a named file that is not there = %v, want an error naming it", err)
	}
}

// still is a time server that says nothing and is never asked.
type still struct{}

func (still) Stats(context.Context) (leontp.Stats, error) { return leontp.Stats{}, nil }
func (still) Clock(context.Context) (leontp.Clock, error) { return leontp.Clock{}, nil }

// The client is made for the server the settings name, asked as they say. No server, or no time to wait for it, is a mistake that is said with
// the variable to set.
func TestReadings(t *testing.T) {
	t.Parallel()

	var server string
	var options leontp.Options
	connect := func(s string, o leontp.Options) (readings.Server, error) {
		server, options = s, o
		return still{}, nil
	}

	f := FlagData{Server: testServer, Timeout: 250 * time.Millisecond, ShortQuestion: true}
	if _, err := f.readings(connect); err != nil {
		t.Fatal(err)
	}

	if want := (leontp.Options{Timeout: 250 * time.Millisecond, ShortQuestion: true}); server != testServer || options != want {
		t.Errorf("the client was made for %q with %+v, want %q with %+v", server, options, testServer, want)
	}

	for name, c := range map[string]struct {
		flags FlagData
		want  string
	}{
		"no server":         {FlagData{Timeout: time.Second}, "LEONTP_METRICS_SERVER"},
		"no time to wait":   {FlagData{Server: testServer}, "LEONTP_METRICS_TIMEOUT"},
		"less than no time": {FlagData{Server: testServer, Timeout: -time.Second}, "LEONTP_METRICS_TIMEOUT"},
		"a client not made": {FlagData{Server: "nowhere", Timeout: time.Second}, "no such host"},
	} {
		_, err := c.flags.readings(func(s string, _ leontp.Options) (readings.Server, error) {
			if s == "nowhere" {
				return nil, errors.New("no such host")
			}

			return still{}, nil
		})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: readings = %v, want an error saying %s", name, err, c.want)
		}
	}
}

// Labels that cannot be put on the page are said with the setting they came from, whether they are written wrong or are names a reading has.
func TestPage(t *testing.T) {
	t.Parallel()

	for labels, want := range map[string]string{
		"":                       "",
		"hostname=ntp,site=home": "",
		"hostname":               "name=value",
		"host-name=ntp":          "host-name",
		"serial=1":               "LEONTP_METRICS_LABELS",
		"version=1":              "LEONTP_METRICS_LABELS",
	} {
		_, err := (&FlagData{Labels: labels}).page(readings.New(still{}))

		switch {
		case want == "" && err != nil:
			t.Errorf("page with the labels %q: %v", labels, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "--labels")):
			t.Errorf("page with the labels %q = %v, want an error naming --labels and saying %q", labels, err, want)
		}
	}
}
