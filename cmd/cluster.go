//go:build !noserver

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/server"
)

const (
	clusterPeerLeftRecently = 10 * time.Minute // A peer gone longer than this was likely removed on purpose
)

func init() {
	commands = append(commands, cmdCluster)
}

var flagsCluster = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: server.DefaultConfigFile, DefaultText: server.DefaultConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-admin-unix", Aliases: []string{"listen_admin_unix", "experimental-listen-admin-unix", "experimental_listen_admin_unix"}, EnvVars: []string{"NTFY_LISTEN_ADMIN_UNIX", "NTFY_EXPERIMENTAL_LISTEN_ADMIN_UNIX"}, Usage: "the server's admin socket, which serves the status"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file", "H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file, whose owner may own the admin socket"}),
)

var cmdCluster = &cli.Command{
	Name:      "cluster",
	Usage:     "Show the state of a cluster node",
	UsageText: "ntfy cluster status [--all] [--json]",
	Flags:     flagsCluster,
	Before:    initConfigFileInputSourceFunc("config", flagsCluster, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "status",
			Usage:     "Shows this node's view of the cluster",
			UsageText: "ntfy cluster status [--all] [--json]",
			Action:    execClusterStatus,
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "json", Usage: "print the status as JSON"},
				&cli.BoolFlag{Name: "all", Usage: "also fetch every live peer's view, and show all links between the nodes"},
			},
			Description: `Show this node's view of the cluster, as served on the server's admin socket: its role and
leadership, and per peer the message batches it sent there and received from there.

A link that only works one way (e.g. a firewall dropping the cluster port in one direction)
shows as failed batches on the sending node and nothing received on the other one; it does
not show in /v1/health. Run this on the node itself, as root or as a member of the server's
group (usually ntfy): it asks the server over the admin socket (listen-admin-unix,
read from the server config).

With --all, the server also fetches every live peer's view and shows each link between the
nodes once, with what the sender sent and what the receiver received.

Examples:
  ntfy cluster status          # Show peers and link problems
  ntfy cluster status --all    # Show all nodes and every link between them
  ntfy cluster status --json   # Print the raw status, e.g. for scripts
`,
		},
	},
}

func execClusterStatus(c *cli.Context) error {
	client, err := newClusterStatusClient(c)
	if err != nil {
		return err
	}
	if c.Bool("all") {
		nodes, err := client.ClusterNodeStatuses()
		if err != nil {
			return clusterStatusError(c, err)
		}
		if c.Bool("json") {
			encoder := json.NewEncoder(c.App.Writer)
			encoder.SetIndent("", "  ")
			return encoder.Encode(nodes)
		}
		printClusterNodeStatuses(c.App.Writer, nodes)
		return nil
	}
	status, err := client.ClusterStatus()
	if err != nil {
		return clusterStatusError(c, err)
	}
	if c.Bool("json") {
		encoder := json.NewEncoder(c.App.Writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(status)
	}
	printClusterStatus(c.App.Writer, status)
	return nil
}

// newClusterStatusClient returns a client for the server's admin socket, which must be enabled
func newClusterStatusClient(c *cli.Context) (*socketClient, error) {
	socket := c.String("listen-admin-unix")
	if socket == "" || socket == "-" {
		return nil, errors.New("the admin socket is not enabled on this server, set listen-admin-unix in the server config (e.g. /run/ntfy/admin.sock) and restart it")
	}
	if err := checkAdminSocketOwner(socket, c.String("auth-file")); err != nil {
		return nil, err
	}
	return newSocketClient(socket), nil
}

// clusterStatusError turns an admin API error into what to do about it
func clusterStatusError(c *cli.Context, err error) error {
	socket := c.String("listen-admin-unix")
	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr) && apiErr.Code == apiErrCodeClusterNotEnabled:
		return errors.New("clustering is not enabled on this node (cluster-listen is not set)")
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("permission denied on the admin socket %s, run this command as root or as a member of the server's group (usually ntfy)", socket)
	case errors.Is(err, errSocketUnreachable):
		return fmt.Errorf("cannot reach the server on its admin socket %s, is it running? (%w)", socket, err)
	}
	return err
}

func printClusterStatus(w io.Writer, status *cluster.Status) {
	// This node first, then its peers; the link columns are the peers' only
	self, live := string(status.NodeID)+" (this node)", "yes"
	if status.Leader {
		self = string(status.NodeID) + " (this node, leader)"
	}
	if !status.Healthy {
		live = "no"
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tROLE\tLIVE\tHEARTBEAT\tSENT OK\tFAILED\tQUEUED\tDROPPED\tLAST SENT\tRECEIVED\tLAST RECEIVED")
	fmt.Fprintf(tw, "%s\t%s\t%s\t-\t-\t-\t-\t-\t-\t-\t-\n", self, status.Role, live)
	for _, p := range status.Peers {
		live, heartbeat := "no", "last seen "+clusterStatusAge(status.Time, p.LastSeen)
		if p.Live {
			live, heartbeat = "yes", fmt.Sprintf("%ds ago", p.HeartbeatAge)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%d\t%s\n", p.NodeID, p.Role, live, heartbeat, p.Out.BatchesOK, p.Out.BatchesFailed,
			p.Out.Queued, p.Out.Dropped, clusterStatusAge(status.Time, p.Out.LastOK), p.In.Batches, clusterStatusAge(status.Time, p.In.LastAt))
	}
	tw.Flush()
	problems := clusterStatusProblems(status, "")
	if !status.Healthy {
		problems = append([]string{fmt.Sprintf("%s (this node): not healthy, its registry heartbeat is stale", status.NodeID)}, problems...)
	}
	if len(problems) == 0 {
		return
	}
	fmt.Fprintln(w, "\nProblems:")
	for _, problem := range problems {
		fmt.Fprintf(w, "  %s\n", problem)
	}
}

// clusterStatusProblems lists what an operator should look at: outbound batches that fail
// (one-way links show up here on the sending side), drops, and peers that left the registry.
// With from set, each line names the link ("from -> peer") instead of only the peer.
func clusterStatusProblems(status *cluster.Status, from cluster.NodeID) []string {
	problems := make([]string, 0)
	for _, p := range status.Peers {
		label := string(p.NodeID)
		if from != "" {
			label = fmt.Sprintf("%s -> %s", from, p.NodeID)
		}
		lastError := fmt.Sprintf("last error %s: %s", clusterStatusAge(status.Time, p.Out.LastErrorAt), p.Out.LastError)
		if p.Out.BatchesFailed > 0 && p.Out.BatchesOK == 0 {
			problems = append(problems, fmt.Sprintf("%s: no batch accepted, %d failed; %s", label, p.Out.BatchesFailed, lastError))
		} else if p.Out.BatchesFailed > 0 && p.Out.LastFailed {
			problems = append(problems, fmt.Sprintf("%s: the last batch failed (%d of %d failed); %s", label, p.Out.BatchesFailed, p.Out.BatchesFailed+p.Out.BatchesOK, lastError))
		}
		if p.Out.Dropped > 0 {
			problems = append(problems, fmt.Sprintf("%s: %d message(s) dropped because its queue was full", label, p.Out.Dropped))
		}
		if !p.Live && (p.LastSeen == 0 || status.Time-p.LastSeen <= int64(clusterPeerLeftRecently.Seconds())) {
			problems = append(problems, fmt.Sprintf("%s: left the registry %s (counters are from before it left)", label, clusterStatusAge(status.Time, p.LastSeen)))
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

// printClusterNodeStatuses prints the nodes, then every link once: what the sender sent, next to
// what the receiver received from it ("?" if the receiver's view is not available)
func printClusterNodeStatuses(w io.Writer, nodes []*cluster.NodeStatus) {
	byID := make(map[cluster.NodeID]*cluster.Status)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tROLE\tLEADER\tHEALTHY")
	for _, node := range nodes {
		if node.Status == nil {
			fmt.Fprintf(tw, "%s\t?\t?\t?\n", node.NodeID)
			continue
		}
		byID[node.NodeID] = node.Status
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", node.NodeID, node.Status.Role, clusterStatusYesNo(node.Status.Leader), clusterStatusYesNo(node.Status.Healthy))
	}
	tw.Flush()
	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "LINK\tSENT OK\tFAILED\tLAST SENT\tRECEIVED\tLAST RECEIVED")
	problems := make([]string, 0)
	for _, node := range nodes {
		if node.Status == nil {
			problems = append(problems, fmt.Sprintf("%s: cannot fetch its status: %s", node.NodeID, node.Error))
			continue
		}
		for _, p := range node.Status.Peers {
			received, lastReceived := "?", "?"
			if peerStatus, ok := byID[p.NodeID]; ok {
				in := clusterStatusInboundFrom(peerStatus, node.NodeID)
				received, lastReceived = fmt.Sprintf("%d", in.Batches), clusterStatusAge(peerStatus.Time, in.LastAt)
			}
			fmt.Fprintf(tw, "%s -> %s\t%d\t%d\t%s\t%s\t%s\n", node.NodeID, p.NodeID, p.Out.BatchesOK, p.Out.BatchesFailed,
				clusterStatusAge(node.Status.Time, p.Out.LastOK), received, lastReceived)
		}
		problems = append(problems, clusterStatusProblems(node.Status, node.NodeID)...)
	}
	tw.Flush()
	if len(problems) == 0 {
		return
	}
	fmt.Fprintln(w, "\nProblems:")
	for _, problem := range problems {
		fmt.Fprintf(w, "  %s\n", problem)
	}
}

// clusterStatusInboundFrom returns what a node received from the given peer, or nothing if it never did
func clusterStatusInboundFrom(status *cluster.Status, from cluster.NodeID) cluster.PeerInbound {
	for _, p := range status.Peers {
		if p.NodeID == from {
			return p.In
		}
	}
	return cluster.PeerInbound{}
}

func clusterStatusYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
