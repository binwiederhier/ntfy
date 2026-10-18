//go:build !noserver

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/server"
)

const (
	clusterStatusTimeout = 10 * time.Second
)

func init() {
	commands = append(commands, cmdCluster)
}

var flagsCluster = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: server.DefaultConfigFile, DefaultText: server.DefaultConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "experimental-cluster-listen", Aliases: []string{"experimental_cluster_listen"}, EnvVars: []string{"NTFY_EXPERIMENTAL_CLUSTER_LISTEN"}, Usage: "ip:port of this node's cluster listener"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "experimental-cluster-secret", Aliases: []string{"experimental_cluster_secret"}, EnvVars: []string{"NTFY_EXPERIMENTAL_CLUSTER_SECRET"}, Usage: "shared secret authenticating cluster requests"}),
)

var cmdCluster = &cli.Command{
	Name:      "cluster",
	Usage:     "Show the state of an experimental cluster node",
	UsageText: "ntfy cluster status [--json]",
	Flags:     flagsCluster,
	Before:    initConfigFileInputSourceFunc("config", flagsCluster, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "status",
			Usage:     "Shows this node's view of the cluster",
			UsageText: "ntfy cluster status [--json]",
			Action:    execClusterStatus,
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "json", Usage: "print the status as JSON"},
			},
			Description: `Show this node's view of the cluster, as served on its cluster listener: its role and
leadership, and per peer the message batches it sent there and received from there.

A link that only works one way (e.g. a firewall dropping the cluster port in one direction)
shows as failed batches on the sending node and nothing received on the other one; it does
not show in /v1/health. Run this on the node itself: it reads experimental-cluster-listen and
experimental-cluster-secret from the server config.

Examples:
  ntfy cluster status          # Show peers and link problems
  ntfy cluster status --json   # Print the raw status, e.g. for scripts
`,
		},
	},
}

func execClusterStatus(c *cli.Context) error {
	listen := c.String("experimental-cluster-listen")
	if listen == "" {
		return errors.New("clustering is not enabled on this node (experimental-cluster-listen is not set)")
	}
	baseURL, err := clusterStatusBaseURL(listen)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterStatusTimeout)
	defer cancel()
	status, err := cluster.FetchStatus(ctx, baseURL, c.String("experimental-cluster-secret"))
	if err != nil {
		return err
	}
	if c.Bool("json") {
		encoder := json.NewEncoder(c.App.Writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(status)
	}
	printClusterStatus(c.App.Writer, status)
	return nil
}

// clusterStatusBaseURL turns the listen address into a URL on this host: a listener bound to all
// interfaces is reached on localhost
func clusterStatusBaseURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid experimental-cluster-listen %q: %w", listen, err)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func printClusterStatus(w io.Writer, status *cluster.Status) {
	leader, healthy := "not leader", "healthy"
	if status.Leader {
		leader = "leader"
	}
	if !status.Healthy {
		healthy = "NOT healthy (registry heartbeat stale)"
	}
	fmt.Fprintf(w, "Node %s: %s, %s, %s\n\n", status.NodeID, status.Role, leader, healthy)
	if len(status.Peers) == 0 {
		fmt.Fprintln(w, "No peers.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PEER\tROLE\tLIVE\tHEARTBEAT\tSENT OK\tFAILED\tQUEUED\tDROPPED\tLAST SENT\tRECEIVED\tLAST RECEIVED")
	for _, p := range status.Peers {
		live, heartbeat := "no", "-"
		if p.Live {
			live, heartbeat = "yes", fmt.Sprintf("%ds ago", p.HeartbeatAge)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%d\t%s\n", p.NodeID, p.Role, live, heartbeat, p.Out.BatchesOK, p.Out.BatchesFailed,
			p.Out.Queued, p.Out.Dropped, clusterStatusAge(status.Time, p.Out.LastOK), p.In.Batches, clusterStatusAge(status.Time, p.In.LastAt))
	}
	tw.Flush()
	problems := clusterStatusProblems(status)
	if len(problems) == 0 {
		return
	}
	fmt.Fprintln(w, "\nProblems:")
	for _, problem := range problems {
		fmt.Fprintf(w, "  %s\n", problem)
	}
}

// clusterStatusProblems lists what an operator should look at: outbound batches that fail
// (one-way links show up here on the sending side), drops, and peers that left the registry
func clusterStatusProblems(status *cluster.Status) []string {
	problems := make([]string, 0)
	for _, p := range status.Peers {
		lastError := fmt.Sprintf("last error %s: %s", clusterStatusAge(status.Time, p.Out.LastErrorAt), p.Out.LastError)
		if p.Out.BatchesFailed > 0 && p.Out.BatchesOK == 0 {
			problems = append(problems, fmt.Sprintf("%s: no batch accepted, %d failed; %s", p.NodeID, p.Out.BatchesFailed, lastError))
		} else if p.Out.BatchesFailed > 0 && p.Out.LastErrorAt > p.Out.LastOK {
			problems = append(problems, fmt.Sprintf("%s: the last batch failed (%d of %d failed); %s", p.NodeID, p.Out.BatchesFailed, p.Out.BatchesFailed+p.Out.BatchesOK, lastError))
		}
		if p.Out.Dropped > 0 {
			problems = append(problems, fmt.Sprintf("%s: %d message(s) dropped because its queue was full", p.NodeID, p.Out.Dropped))
		}
		if !p.Live {
			problems = append(problems, fmt.Sprintf("%s: no longer in the registry (counters are from before it left)", p.NodeID))
		}
	}
	return problems
}

// clusterStatusAge renders a status timestamp relative to the time the status was taken
func clusterStatusAge(now, t int64) string {
	if t == 0 {
		return "never"
	}
	return (time.Duration(now-t) * time.Second).String() + " ago"
}
