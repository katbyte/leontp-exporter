package cli

import (
	"os/signal"
	"syscall"

	"github.com/katbyte/go-kt/pmetrics"
	"github.com/spf13/cobra"
)

func serveCmd(connect connector) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve the metrics page",
		Long: `Serves the page at /metrics in the Prometheus text format, with a health probe at /healthz.

The time server is asked each time the page is read, so a scrape holds what it said a
moment ago. A server that does not answer within --timeout is asked once more, and is then
reported as leontp_up 0 with the rest of its readings left off.

--labels puts labels of your choosing on every reading, hostname=ntp say, for a store that
tells one source from another by them.

Every setting is also an environment variable: LEONTP_METRICS_SERVER, LEONTP_METRICS_LISTEN,
LEONTP_METRICS_LABELS, LEONTP_METRICS_TIMEOUT and LEONTP_METRICS_SHORT_QUESTION.`,
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

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return pmetrics.Serve(ctx, f.Listen, p.Handler())
		},
	}
}
