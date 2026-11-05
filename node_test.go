package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tcpHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// 127.0.0.1:8080 LISTEN, 10.0.0.2:40000 -> 10.0.0.9:6379 ESTABLISHED, 10.0.0.2:40001 -> 10.0.0.9:6379 TIME_WAIT
const podTCP = tcpHeader +
	"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1001 1 0000000000000000 100 0 0 10 0\n" +
	"   1: 0200000A:9C40 0900000A:18EB 01 00000000:00000000 00:00000000 00000000     0        0 1002 1 0000000000000000 20 4 30 10 -1\n" +
	"   2: 0200000A:9C41 0900000A:18EB 06 00000000:00000000 00:00000000 00000000     0        0 0 3 0000000000000000\n"

// [::ffff:10.0.0.2]:8080 <- [::ffff:10.0.0.7]:51000 ESTABLISHED (inbound)
const podTCP6 = "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
	"   0: 0000000000000000FFFF00000200000A:1F90 0000000000000000FFFF00000700000A:C738 01 00000000:00000000 00:00000000 00000000     0        0 2001 1 0000000000000000 20 4 30 10 -1\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeProc(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sys/net/ipv4/ip_local_port_range"), "32768\t60999\n")

	// pod A (systemd cgroup driver): pause + app container, only IPv4 and IPv6 files
	writeFile(t, filepath.Join(root, "100/cgroup"), "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podaaaaaaaa_1111_2222_3333_444444444444.slice/cri-containerd-abc.scope\n")
	writeFile(t, filepath.Join(root, "100/net/tcp"), podTCP)
	writeFile(t, filepath.Join(root, "100/net/tcp6"), podTCP6)
	writeFile(t, filepath.Join(root, "101/cgroup"), "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podaaaaaaaa_1111_2222_3333_444444444444.slice/cri-containerd-def.scope\n")

	// pod B (cgroupfs driver): IPv4 only
	writeFile(t, filepath.Join(root, "200/cgroup"), "12:memory:/kubepods/besteffort/podbbbbbbbb-1111-2222-3333-444444444444/xyz\n")
	writeFile(t, filepath.Join(root, "200/net/tcp"), tcpHeader)

	// a host process outside any Pod
	writeFile(t, filepath.Join(root, "1/cgroup"), "0::/init.scope\n")
	return root
}

func TestPodUIDFromCgroup(t *testing.T) {
	cases := map[string]string{
		"0::/kubepods.slice/kubepods-pod12345678_abcd_ef01_2345_6789abcdef01.slice/cri-containerd-x.scope": "12345678-abcd-ef01-2345-6789abcdef01",
		"11:cpu:/kubepods/burstable/pod12345678-abcd-ef01-2345-6789abcdef01/0123":                          "12345678-abcd-ef01-2345-6789abcdef01",
		"0::/system.slice/containerd.service":                                                              "",
	}
	for in, want := range cases {
		if got := podUIDFromCgroup([]byte(in)); got != want {
			t.Errorf("podUIDFromCgroup(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollectNodeMetrics(t *testing.T) {
	procRoot = fakeProc(t)
	defer func() { procRoot = "/proc" }()

	pods := map[string]podInfo{
		"aaaaaaaa-1111-2222-3333-444444444444": {Namespace: "monaka", Name: "monaka-1", App: "monaka"},
		"bbbbbbbb-1111-2222-3333-444444444444": {Namespace: "gulab", Name: "gulab-1", App: "gulab"},
		"cccccccc-1111-2222-3333-444444444444": {Namespace: "gulab", Name: "gone", App: "gulab"},
	}
	out := collectNodeMetrics(pods)

	a := `namespace="monaka",pod_name="monaka-1",app="monaka"`
	for _, want := range []string{
		`port_total{` + a + `} 28232`,
		`port_used{` + a + `,remote_addr="0.0.0.0 [0:unknown]",state="LISTEN"} 1`,
		`port_used{` + a + `,remote_addr="10.0.0.9 [6379:redis]",state="ESTABLISHED"} 1`,
		`port_used{` + a + `,remote_addr="10.0.0.9 [6379:redis]",state="TIME_WAIT"} 1`,
		`port_used{` + a + `,remote_addr="OTHER",state="ESTABLISHED"} 1`, // from tcp6
		`port_total{namespace="gulab",pod_name="gulab-1",app="gulab"} 28232`,
		`port_collector_pods{result="collected"} 2`,
		`port_collector_pods{result="missing"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Count(out, "# TYPE port_used gauge") != 1 {
		t.Errorf("expected one TYPE line per metric family:\n%s", out)
	}
}
