// Package cli implements the leontp-metrics command line interface: the cobra commands, flag and config handling, and the server that serves the
// metrics page.
package cli

import (
	"errors"
	"fmt"

	"github.com/katbyte/go-kt/version"
	"github.com/spf13/cobra"
)

func Make() (*cobra.Command, error) {
	return makeWith(connect)
}

// makeWith is Make with the time server's client made by connect, so a test can stand in for a time server.
func makeWith(connect connector) (*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "leontp-metrics [command]",
		Short: "leontp-metrics reports what a LeoNTP time server says about itself as a Prometheus metrics page",
		Long: `Reports what a LeoNTP time server says about itself as a Prometheus metrics page: how long it
has been up and locked to GPS, how many satellites it is using, how many requests it has
answered, and the stratum and leap warning it gives its clients. It only ever asks the
server questions, over its NTP port.
Complete documentation is available at https://github.com/katbyte/leontp-metrics`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run \"leontp-metrics help\" for more information about available leontp-metrics commands.")
			return nil
		},
	}

	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "Print the version number of leontp-metrics",
		Long:          `Print the version number of leontp-metrics`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "leontp-metrics "+version.Version)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "print",
		Short: "Ask the time server once and print the metrics page",
		Long: `Asks the time server both questions once and prints the page a scrape would read, without
serving anything: a check that the address is right and the server answers, and a look at
what would be reported.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			f := GetFlags()
			r, err := f.readings(connect)
			if err != nil {
				return err
			}

			p, err := f.page(r)
			if err != nil {
				return err
			}

			if err := p.Write(cmd.OutOrStdout()); err != nil {
				return err
			}

			// the page is printed either way, with what a server that did not answer looks like on it; what went wrong is in the log
			if !r.Answering() {
				return errors.New("the time server did not answer every question")
			}

			return nil
		},
	})

	root.AddCommand(serveCmd(connect))

	if err := configureFlags(root); err != nil {
		return nil, fmt.Errorf("unable to configure flags: %w", err)
	}
	// the settings file is read once the flags are parsed, since a flag can name it; before any command's own checks, which read what it holds
	root.PersistentPreRunE = func(*cobra.Command, []string) error { return loadConfig() }

	return root, nil
}
