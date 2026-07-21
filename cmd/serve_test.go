package cmd

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/test"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestParseUsers_Success(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []*user.User
	}{
		{
			name:  "single user",
			input: []string{"alice:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user"},
			expected: []*user.User{
				{
					Name:        "alice",
					Hash:        "$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S",
					Role:        user.RoleUser,
					Provisioned: true,
				},
			},
		},
		{
			name: "multiple users with different roles",
			input: []string{
				"alice:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user",
				"bob:$2a$10$jIcuBWcbxd6oW1aPvoJ5iOShzu3/UJ2kSxKbTZtDypG06nBflQagq:admin",
			},
			expected: []*user.User{
				{
					Name:        "alice",
					Hash:        "$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S",
					Role:        user.RoleUser,
					Provisioned: true,
				},
				{
					Name:        "bob",
					Hash:        "$2a$10$jIcuBWcbxd6oW1aPvoJ5iOShzu3/UJ2kSxKbTZtDypG06nBflQagq",
					Role:        user.RoleAdmin,
					Provisioned: true,
				},
			},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []*user.User{},
		},
		{
			name:  "user with special characters in name",
			input: []string{"alice.test+123@example.com:$2a$10$RYUYAsl5zOnAIp6fH7BPX.Eug0rUfEUk92r8WiVusb0VK.vGojWBe:user"},
			expected: []*user.User{
				{
					Name:        "alice.test+123@example.com",
					Hash:        "$2a$10$RYUYAsl5zOnAIp6fH7BPX.Eug0rUfEUk92r8WiVusb0VK.vGojWBe",
					Role:        user.RoleUser,
					Provisioned: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseUsers(tt.input)
			require.NoError(t, err)
			require.Len(t, result, len(tt.expected))

			for i, expectedUser := range tt.expected {
				assert.Equal(t, expectedUser.Name, result[i].Name)
				assert.Equal(t, expectedUser.Hash, result[i].Hash)
				assert.Equal(t, expectedUser.Role, result[i].Role)
				assert.Equal(t, expectedUser.Provisioned, result[i].Provisioned)
			}
		})
	}
}

func TestParseUsers_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		error string
	}{
		{
			name:  "invalid format - too few parts",
			input: []string{"alice:hash"},
			error: "invalid auth-users: alice:hash, expected format: 'name:hash:role'",
		},
		{
			name:  "invalid format - too many parts",
			input: []string{"alice:hash:role:extra"},
			error: "invalid auth-users: alice:hash:role:extra, expected format: 'name:hash:role'",
		},
		{
			name:  "invalid username",
			input: []string{"alice@#$%:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user"},
			error: "invalid auth-users: alice@#$%:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user, username invalid",
		},
		{
			name:  "invalid password hash - wrong prefix",
			input: []string{"alice:plaintext:user"},
			error: "invalid auth-users: alice:plaintext:user, password hash invalid, password hash must be a bcrypt hash, use 'ntfy user hash' to generate",
		},
		{
			name:  "invalid role",
			input: []string{"alice:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:invalid"},
			error: "invalid auth-users: alice:$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:invalid, role invalid is not allowed, allowed roles are 'admin' or 'user'",
		},
		{
			name:  "empty username",
			input: []string{":$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user"},
			error: "invalid auth-users: :$2a$10$320YlQeaMghYZsvtu9jzfOQZS32FysWY/T9qu5NWqcIh.DN.u5P5S:user, username invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseUsers(tt.input)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), tt.error)
		})
	}
}

func TestParseAccess_Success(t *testing.T) {
	users := []*user.User{
		{Name: "alice", Role: user.RoleUser},
		{Name: "bob", Role: user.RoleUser},
	}

	tests := []struct {
		name     string
		users    []*user.User
		input    []string
		expected map[string][]*user.Grant
	}{
		{
			name:  "single access entry",
			users: users,
			input: []string{"alice:mytopic:read-write"},
			expected: map[string][]*user.Grant{
				"alice": {
					{
						TopicPattern: "mytopic",
						Permission:   user.PermissionReadWrite,
						Provisioned:  true,
					},
				},
			},
		},
		{
			name:  "multiple access entries for same user",
			users: users,
			input: []string{
				"alice:topic1:read-only",
				"alice:topic2:write-only",
			},
			expected: map[string][]*user.Grant{
				"alice": {
					{
						TopicPattern: "topic1",
						Permission:   user.PermissionRead,
						Provisioned:  true,
					},
					{
						TopicPattern: "topic2",
						Permission:   user.PermissionWrite,
						Provisioned:  true,
					},
				},
			},
		},
		{
			name:  "access for everyone",
			users: users,
			input: []string{"everyone:publictopic:read-only"},
			expected: map[string][]*user.Grant{
				user.Everyone: {
					{
						TopicPattern: "publictopic",
						Permission:   user.PermissionRead,
						Provisioned:  true,
					},
				},
			},
		},
		{
			name:  "wildcard topic pattern",
			users: users,
			input: []string{"alice:topic*:read-write"},
			expected: map[string][]*user.Grant{
				"alice": {
					{
						TopicPattern: "topic*",
						Permission:   user.PermissionReadWrite,
						Provisioned:  true,
					},
				},
			},
		},
		{
			name:     "empty input",
			users:    users,
			input:    []string{},
			expected: map[string][]*user.Grant{},
		},
		{
			name:  "deny-all permission",
			users: users,
			input: []string{"alice:secretopic:deny-all"},
			expected: map[string][]*user.Grant{
				"alice": {
					{
						TopicPattern: "secretopic",
						Permission:   user.PermissionDenyAll,
						Provisioned:  true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAccess(tt.users, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAccess_Errors(t *testing.T) {
	users := []*user.User{
		{Name: "alice", Role: user.RoleUser},
		{Name: "admin", Role: user.RoleAdmin},
	}

	tests := []struct {
		name  string
		users []*user.User
		input []string
		error string
	}{
		{
			name:  "invalid format - too few parts",
			users: users,
			input: []string{"alice:topic"},
			error: "invalid auth-access: alice:topic, expected format: 'user:topic:permission'",
		},
		{
			name:  "invalid format - too many parts",
			users: users,
			input: []string{"alice:topic:read:extra"},
			error: "invalid auth-access: alice:topic:read:extra, expected format: 'user:topic:permission'",
		},
		{
			name:  "user not provisioned",
			users: users,
			input: []string{"charlie:topic:read"},
			error: "invalid auth-access: charlie:topic:read, user charlie is not provisioned",
		},
		{
			name:  "admin user cannot have ACL entries",
			users: users,
			input: []string{"admin:topic:read"},
			error: "invalid auth-access: admin:topic:read, user admin is not a regular user, only regular users can have ACL entries",
		},
		{
			name:  "invalid topic pattern",
			users: users,
			input: []string{"alice:topic-with-invalid-chars!:read"},
			error: "invalid auth-access: alice:topic-with-invalid-chars!:read, topic pattern topic-with-invalid-chars! invalid",
		},
		{
			name:  "invalid permission",
			users: users,
			input: []string{"alice:topic:invalid-permission"},
			error: "invalid auth-access: alice:topic:invalid-permission, permission invalid-permission invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAccess(tt.users, tt.input)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), tt.error)
		})
	}
}

func TestParseTokens_Success(t *testing.T) {
	users := []*user.User{
		{Name: "alice"},
		{Name: "bob"},
	}

	tests := []struct {
		name     string
		users    []*user.User
		input    []string
		expected map[string][]*user.Token
	}{
		{
			name:  "single token without label",
			users: users,
			input: []string{"alice:tk_abcdefghijklmnopqrstuvwxyz123"},
			expected: map[string][]*user.Token{
				"alice": {
					{
						Value:       "tk_abcdefghijklmnopqrstuvwxyz123",
						Label:       "",
						Provisioned: true,
					},
				},
			},
		},
		{
			name:  "single token with label",
			users: users,
			input: []string{"alice:tk_abcdefghijklmnopqrstuvwxyz123:My Phone"},
			expected: map[string][]*user.Token{
				"alice": {
					{
						Value:       "tk_abcdefghijklmnopqrstuvwxyz123",
						Label:       "My Phone",
						Provisioned: true,
					},
				},
			},
		},
		{
			name:  "multiple tokens for same user",
			users: users,
			input: []string{
				"alice:tk_abcdefghijklmnopqrstuvwxyz123:Phone",
				"alice:tk_zyxwvutsrqponmlkjihgfedcba987:Laptop",
			},
			expected: map[string][]*user.Token{
				"alice": {
					{
						Value:       "tk_abcdefghijklmnopqrstuvwxyz123",
						Label:       "Phone",
						Provisioned: true,
					},
					{
						Value:       "tk_zyxwvutsrqponmlkjihgfedcba987",
						Label:       "Laptop",
						Provisioned: true,
					},
				},
			},
		},
		{
			name:  "tokens for multiple users",
			users: users,
			input: []string{
				"alice:tk_abcdefghijklmnopqrstuvwxyz123:Phone",
				"bob:tk_zyxwvutsrqponmlkjihgfedcba987:Tablet",
			},
			expected: map[string][]*user.Token{
				"alice": {
					{
						Value:       "tk_abcdefghijklmnopqrstuvwxyz123",
						Label:       "Phone",
						Provisioned: true,
					},
				},
				"bob": {
					{
						Value:       "tk_zyxwvutsrqponmlkjihgfedcba987",
						Label:       "Tablet",
						Provisioned: true,
					},
				},
			},
		},
		{
			name:     "empty input",
			users:    users,
			input:    []string{},
			expected: map[string][]*user.Token{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTokens(tt.users, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTokens_Errors(t *testing.T) {
	users := []*user.User{
		{Name: "alice"},
	}

	tests := []struct {
		name  string
		users []*user.User
		input []string
		error string
	}{
		{
			name:  "invalid format - too few parts",
			users: users,
			input: []string{"alice"},
			error: "invalid auth-tokens: alice, expected format: 'user:token[:label]'",
		},
		{
			name:  "invalid format - too many parts",
			users: users,
			input: []string{"alice:token:label:extra:parts"},
			error: "invalid auth-tokens: alice:token:label:extra:parts, expected format: 'user:token[:label]'",
		},
		{
			name:  "user not provisioned",
			users: users,
			input: []string{"charlie:tk_abcdefghijklmnopqrstuvwxyz123"},
			error: "invalid auth-tokens: charlie:tk_abcdefghijklmnopqrstuvwxyz123, user charlie is not provisioned",
		},
		{
			name:  "invalid token format",
			users: users,
			input: []string{"alice:invalid-token"},
			error: "invalid auth-tokens: alice:invalid-token, token invalid-token invalid, use 'ntfy token generate' to generate a random token",
		},
		{
			name:  "token too short",
			users: users,
			input: []string{"alice:tk_short"},
			error: "invalid auth-tokens: alice:tk_short, token tk_short invalid, use 'ntfy token generate' to generate a random token",
		},
		{
			name:  "token without prefix",
			users: users,
			input: []string{"alice:abcdefghijklmnopqrstuvwxyz12345"},
			error: "invalid auth-tokens: alice:abcdefghijklmnopqrstuvwxyz12345, token abcdefghijklmnopqrstuvwxyz12345 invalid, use 'ntfy token generate' to generate a random token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTokens(tt.users, tt.input)
			require.Error(t, err)
			require.Nil(t, result)
			assert.Contains(t, err.Error(), tt.error)
		})
	}
}

func TestCLI_Serve_Unix_Curl(t *testing.T) {
	sockFile := filepath.Join(t.TempDir(), "ntfy.sock")
	configFile := newEmptyFile(t) // Avoid issues with existing server.yml file on system
	go func() {
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", "--config=" + configFile, "--listen-http=-", "--listen-unix=" + sockFile})
		require.Nil(t, err)
	}()
	for i := 0; i < 40 && !util.FileExists(sockFile); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, util.FileExists(sockFile))

	cmd := exec.Command("curl", "-s", "--unix-socket", sockFile, "-d", "this is a message", "localhost/mytopic")
	out, err := cmd.Output()
	require.Nil(t, err)
	m := toMessage(t, string(out))
	require.Equal(t, "this is a message", m.Message)
}

func TestCLI_Serve_WebSocket(t *testing.T) {
	port := 10000 + rand.Intn(20000)
	go func() {
		configFile := newEmptyFile(t) // Avoid issues with existing server.yml file on system
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", "--config=" + configFile, fmt.Sprintf("--listen-http=:%d", port)})
		require.Nil(t, err)
	}()
	test.WaitForPortUp(t, port)

	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/mytopic/ws", port), nil)
	require.Nil(t, err)

	messageType, data, err := ws.ReadMessage()
	require.Nil(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, "open", toMessage(t, string(data)).Event)

	c := client.New(client.NewConfig())
	_, err = c.Publish(fmt.Sprintf("http://127.0.0.1:%d/mytopic", port), "my message")
	require.Nil(t, err)

	messageType, data, err = ws.ReadMessage()
	require.Nil(t, err)
	require.Equal(t, websocket.TextMessage, messageType)

	m := toMessage(t, string(data))
	require.Equal(t, "my message", m.Message)
	require.Equal(t, "mytopic", m.Topic)
}

func TestIP_Host_Parsing(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1":          "1.1.1.1/32",
		"fd00::1234":       "fd00::1234/128",
		"192.168.0.3/24":   "192.168.0.0/24",
		"10.1.2.3/8":       "10.0.0.0/8",
		"201:be93::4a6/21": "201:b800::/21",
	}
	for q, expectedAnswer := range cases {
		ips, err := parseIPHostPrefix(q)
		require.Nil(t, err)
		assert.Equal(t, 1, len(ips))
		assert.Equal(t, expectedAnswer, ips[0].String())
	}
}

func TestCLI_Serve_ClusterModeValidation(t *testing.T) {
	configFile := newEmptyFile(t) // Avoid issues with existing server.yml file on system
	// cluster-mode requires database-url
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "serve", "--config=" + configFile, "--cluster-mode"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "database-url")
	// cluster-mode requires cluster-secret; must fail validation before any database connection
	app, _, _, _ = newTestApp()
	err = app.Run([]string{"ntfy", "serve", "--config=" + configFile, "--cluster-mode", "--database-url=postgres://user:pass@localhost:1/na"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cluster-secret")
}

func newEmptyFile(t *testing.T) string {
	filename := filepath.Join(t.TempDir(), "empty")
	require.Nil(t, os.WriteFile(filename, []byte{}, 0600))
	return filename
}
