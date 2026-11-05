package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
)

// Matches the Pod UID in both cgroup layouts:
//
//	cgroupfs: /kubepods/burstable/pod1b2c3d4e-..../<container>
//	systemd:  /kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1b2c3d4e_....slice/...
var podUIDRegexp = regexp.MustCompile(`pod([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12})`)

func podUIDFromCgroup(cgroup []byte) string {
	m := podUIDRegexp.FindSubmatch(cgroup)
	if m == nil {
		return ""
	}
	return strings.ReplaceAll(string(m[1]), "_", "-")
}

// findPodPIDs walks procRoot and returns one PID per wanted Pod UID. All
// containers in a Pod share one network namespace, so any of its PIDs will do.
func findPodPIDs(wanted map[string]podInfo) map[string]int {
	pids := map[string]int{}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		log.Println("Failed to read", procRoot, err)
		return pids
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cgroup, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cgroup"))
		if err != nil {
			continue // the process exited
		}
		uid := podUIDFromCgroup(cgroup)
		if _, ok := wanted[uid]; !ok {
			continue
		}
		if _, ok := pids[uid]; !ok {
			pids[uid] = pid
		}
		if len(pids) == len(wanted) {
			break
		}
	}
	return pids
}

// readPodConns reads IPv4 and IPv6 connections from the network namespace of pid.
func readPodConns(pid int) (procfs.NetTCP, error) {
	fs, err := procfs.NewFS(filepath.Join(procRoot, strconv.Itoa(pid)))
	if err != nil {
		return nil, err
	}
	tcp, err := fs.NetTCP()
	if err != nil {
		return nil, err
	}
	// tcp6 is missing when IPv6 is disabled in the kernel
	tcp6, err := fs.NetTCP6()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return append(tcp, tcp6...), nil
}

type metricFamily struct {
	help, name string
	samples    strings.Builder
}

func (f *metricFamily) add(labels string, value interface{}) {
	fmt.Fprintf(&f.samples, "%s{%s} %v\n", f.name, labels, value)
}

func (f *metricFamily) writeTo(b *strings.Builder) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n", f.name, f.help, f.name)
	b.WriteString(f.samples.String())
}

func nodeMetricsHandler(cache *podCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(collectNodeMetrics(cache.snapshot())))
	}
}

func collectNodeMetrics(pods map[string]podInfo) string {
	portTotalCount := getPortTotalCount()

	total := &metricFamily{name: "port_total", help: "Total Local Port Count"}
	used := &metricFamily{name: "port_used", help: "Used Local Port Count"}
	usage := &metricFamily{name: "port_usage", help: "Local Port Usage Percentage"}
	collector := &metricFamily{name: "port_collector_pods", help: "Pods on this node by collection result"}

	pids := findPodPIDs(pods)
	collected, missing := 0, 0
	for uid, p := range pods {
		pid, ok := pids[uid]
		if !ok {
			missing++
			continue
		}
		conns, err := readPodConns(pid)
		if err != nil {
			log.Printf("Failed to read connections of %s/%s (pid %d): %v", p.Namespace, p.Name, pid, err)
			missing++
			continue
		}
		collected++

		podLabels := fmt.Sprintf(`namespace="%s",pod_name="%s",app="%s"`, p.Namespace, p.Name, p.App)
		portUsedCount := 0
		for remoteAddr, m := range groupConnections(conns) {
			for state, v := range m {
				portUsedCount += v
				used.add(fmt.Sprintf(`%s,remote_addr="%s",state="%s"`, podLabels, remoteAddr, state), v)
			}
		}

		total.add(podLabels, portTotalCount)
		usage.add(podLabels, fmt.Sprintf("%f", float32(portUsedCount)*100/float32(portTotalCount)))
	}
	collector.add(`result="collected"`, collected)
	collector.add(`result="missing"`, missing)

	var b strings.Builder
	for _, f := range []*metricFamily{total, used, usage, collector} {
		f.writeTo(&b)
	}
	return b.String()
}
