//go:build !noserver

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/server"
)

const (
	clusterStatusTimeout     = 10 * time.Second
	clusterStatusPeerTimeout = 5 * time.Second
	clusterPeerLeftRecently  = 10 * time.Minute // A peer gone longer than this was likely removed on purpose
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
			Description: `Show this node's view of the cluster, as served on its cluster listener: its role and
leadership, and per peer the message batches it sent there and received from there.

A link that only works one way (e.g. a firewall dropping the cluster port in one direction)
shows as failed batches on the sending node and nothing received on the other one; it does
not show in /v1/health. Run this on the node itself: it reads experimental-cluster-listen and
experimental-cluster-secret from the server config.

With --all, it also fetches every live peer's view (from its advertised cluster URL) and shows
each link between the nodes once, with what the sender sent and what the receiver received.

Examples:
  ntfy cluster status          # Show peers and link problems
  ntfy cluster status --all    # Show all nodes and every link between them
  ntfy cluster status --json   # Print the raw status, e.g. for scripts
`,
		},
	},
}

// clusterNodeStatus is one node's view in "status --all": its status, or why it could not be fetched
type clusterNodeStatus struct {
	NodeID cluster.NodeID  `json:"node_id"`
	Status *cluster.Status `json:"status,omitempty"`
	Error  string          `json:"error,omitempty"`
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
	status, err := cluster.NewClient(c.String("experimental-cluster-secret"), "").Status(ctx, baseURL)
	if err != nil {
		return err
	}
	if c.Bool("all") {
		nodes := fetchClusterNodeStatuses(status, c.String("experimental-cluster-secret"))
		if c.Bool("json") {
			encoder := json.NewEncoder(c.App.Writer)
			encoder.SetIndent("", "  ")
			return encoder.Encode(nodes)
		}
		printClusterNodeStatuses(c.App.Writer, nodes)
		return nil
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
		live, heartbeat := "no", "last seen "+clusterStatusAge(status.Time, p.LastSeen)
		if p.Live {
			live, heartbeat = "yes", fmt.Sprintf("%ds ago", p.HeartbeatAge)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%d\t%s\n", p.NodeID, p.Role, live, heartbeat, p.Out.BatchesOK, p.Out.BatchesFailed,
			p.Out.Queued, p.Out.Dropped, clusterStatusAge(status.Time, p.Out.LastOK), p.In.Batches, clusterStatusAge(status.Time, p.In.LastAt))
	}
	tw.Flush()
	problems := clusterStatusProblems(status, "")
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

// fetchClusterNodeStatuses fetches every live peer's view next to this node's, in parallel. A peer
// that does not answer is reported in its entry rather than failing the command.
func fetchClusterNodeStatuses(local *cluster.Status, secret string) []*clusterNodeStatus {
	nodes := []*clusterNodeStatus{{NodeID: local.NodeID, Status: local}}
	client := cluster.NewClient(secret, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, p := range local.Peers {
		if !p.Live || p.AdvertiseURL == "" {
			continue
		}
		wg.Add(1)
		go func(p *cluster.PeerStatus) {
			defer wg.Done()
			node := &clusterNodeStatus{NodeID: p.NodeID}
			ctx, cancel := context.WithTimeout(context.Background(), clusterStatusPeerTimeout)
			defer cancel()
			if status, err := client.Status(ctx, p.AdvertiseURL); err != nil {
				node.Error = err.Error()
			} else {
				node.Status = status
			}
			mu.Lock()
			defer mu.Unlock()
			nodes = append(nodes, node)
		}(p)
	}
	wg.Wait()
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})
	return nodes
}

// printClusterNodeStatuses prints the nodes, then every link once: what the sender sent, next to
// what the receiver received from it ("?" if the receiver's view is not available)
func printClusterNodeStatuses(w io.Writer, nodes []*clusterNodeStatus) {
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
