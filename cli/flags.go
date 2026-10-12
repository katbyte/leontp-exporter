package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/metrics/page"
	"github.com/katbyte/leontp-metrics/lib/readings"
	"github.com/katbyte/leontp-metrics/sdk/leontp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// toolName is what the program is called.
const toolName = "leontp-metrics"

// defaultListen is the NTP port with a 9 in front.
const defaultListen = ":9123"

type FlagData struct {
	Server        string        `mapstructure:"server"`
	Listen        string        `mapstructure:"listen"`
	Labels        string        `mapstructure:"labels"`
	Timeout       time.Duration `mapstructure:"timeout"`
	ShortQuestion bool          `mapstructure:"short-question"`
}

// envNames binds each flag to its environment variable.
var envNames = map[string]string{
	"server":         "LEONTP_METRICS_SERVER",
	"listen":         "LEONTP_METRICS_LISTEN",
	"labels":         "LEONTP_METRICS_LABELS",
	"timeout":        "LEONTP_METRICS_TIMEOUT",
	"short-question": "LEONTP_METRICS_SHORT_QUESTION",
}

// persistent is the flag set configureFlags bound, for the one setting that has to be read before viper is: where the settings file is.
var persistent *pflag.FlagSet

func configureFlags(root *cobra.Command) error {
	pflags := root.PersistentFlags()

	pflags.String("server", "", "the LeoNTP's name or address, e.g. 10.0.0.10; the NTP port is assumed unless one is given")
	pflags.String("listen", defaultListen, "serve the metrics page on this address")
	pflags.String("labels", "", "labels to put on every reading, written name=value,name=value, e.g. hostname=ntp,site=home")
	pflags.Duration("timeout", leontp.DefaultTimeout, "how long to wait for the time server to answer a question before asking once more")
	pflags.Bool("short-question", false, "ask for statistics in the 8 bytes the maker's script sends, for the older LeoNTP; a 1200 does not answer it")
	pflags.String("config", "", "read settings from this file and no other, in place of ./.leontp-metrics or ~/.leontp-metrics (or export LEONTP_METRICS_CONFIG)")

	persistent = pflags

	for name, env := range envNames {
		if err := viper.BindPFlag(name, pflags.Lookup(name)); err != nil {
			return fmt.Errorf("error binding '%s' flag: %w", name, err)
		}

		if err := viper.BindEnv(name, env); err != nil {
			return fmt.Errorf("error binding '%s' to env '%s' : %w", name, env, err)
		}
	}

	return nil
}

// configEnv names the settings file from the environment, as --config does on the command line.
const configEnv = "LEONTP_METRICS_CONFIG"

// configPath is the settings file a run was told to read, "" for none: the flag's, then the environment's. A leading ~/ is the home directory, for
// whatever starts the program without a shell to expand it.
func configPath() string {
	path := os.Getenv(configEnv)
	if persistent != nil {
		if fl := persistent.Lookup("config"); fl != nil && fl.Changed {
			path = fl.Value.String()
		}
	}

	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, rest)
		}
	}

	return path
}

// loadConfig reads the settings file, once the flags are parsed: the one --config or LEONTP_METRICS_CONFIG names and no other, or else the first
// .leontp-metrics found in the working directory and then in $HOME. A file that was named and cannot be read is an error; one merely absent from
// the usual places is not.
func loadConfig() error {
	viper.SetConfigType("env")
	if path := configPath(); path != "" {
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("reading the settings file %s (--config / %s): %w", path, configEnv, err)
		}
	} else {
		viper.SetConfigName("." + toolName)
		// viper reads the first file it finds, so the working directory comes first
		viper.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(home)
		}

		if err := viper.ReadInConfig(); err != nil {
			if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
				clog.Log.Errorf("Error reading config file: %v", err)
			}
		}
	}

	// a settings file spells a setting the way the environment does, less the prefix (SHORT_QUESTION for --short-question), and viper only matches a
	// key spelled as the flag is. Carried across as defaults, they still lose to a flag or the environment.
	for name := range envNames {
		if alt := strings.ReplaceAll(name, "-", "_"); alt != name && viper.InConfig(alt) {
			viper.SetDefault(name, viper.Get(alt))
		}
	}

	return nil
}

// GetFlags returns the fully populated FlagData. It is read from viper rather than from the flags themselves, since viper is what lays the
// environment and the settings file under the command line.
func GetFlags() *FlagData {
	var f FlagData
	if err := viper.Unmarshal(&f); err != nil {
		clog.Log.Fatalf("failed to unmarshal configuration: %v", err)
	}

	return &f
}

// connector makes the client that asks the time server its questions.
type connector func(server string, o leontp.Options) (readings.Server, error)

// readings is what the time server the settings name says about itself, not yet asked. Naming no server is a mistake said now rather than a page
// that only ever says the server is down.
func (f *FlagData) readings(connect connector) (*readings.Readings, error) {
	if f.Server == "" {
		return nil, fmt.Errorf("no time server to ask: set --server (%s)", envNames["server"])
	}

	if f.Timeout <= 0 {
		return nil, fmt.Errorf("--timeout (%s) has to be more than nothing, not %s", envNames["timeout"], f.Timeout)
	}

	server, err := connect(f.Server, leontp.Options{Timeout: f.Timeout, ShortQuestion: f.ShortQuestion})
	if err != nil {
		return nil, err
	}

	clog.Log.Infof("reporting on the time server at %s", f.Server)

	return readings.New(server), nil
}

// page is the metrics page of these readings, with the labels the settings ask for on every one. A label that cannot be put on is a mistake said
// now, with the setting it came from.
func (f *FlagData) page(r *readings.Readings) (*page.Page, error) {
	labels, err := page.ParseLabels(f.Labels)
	if err != nil {
		return nil, fmt.Errorf("--labels (%s): %w", envNames["labels"], err)
	}

	p, err := page.New(page.Options{Name: toolName, Namespace: readings.Namespace, Labels: labels}, r)
	if err != nil {
		return nil, fmt.Errorf("--labels (%s): %w", envNames["labels"], err)
	}

	return p, nil
}
