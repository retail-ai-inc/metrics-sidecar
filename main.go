// MIT License

// Copyright (c) The RAI Authors

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/prometheus/procfs"
)

var portNameMap map[uint64]string = map[uint64]string{
	7:     "echo",
	20:    "ftp-data",
	21:    "ftp",
	22:    "ssh",
	23:    "telnet",
	25:    "smtp",
	53:    "dns",
	66:    "oracle-sql-net",
	67:    "dhcp",
	68:    "dhcp",
	69:    "tftp",
	80:    "http",
	88:    "kerberos",
	110:   "pop3",
	143:   "imap",
	443:   "https",
	464:   "kerberos",
	465:   "smtp-ssl",
	523:   "ibm-db2",
	587:   "smtp",
	993:   "imap-ssl",
	995:   "pop3-ssl",
	1080:  "socks-proxy",
	1194:  "openvpn",
	1433:  "sql-server-2000",
	1944:  "sql-server-7",
	2483:  "oracle-db",
	2484:  "oracle-db",
	3128:  "http-proxy",
	3306:  "mysql",
	3389:  "rdp",
	5432:  "postgres",
	6379:  "redis",
	6665:  "irc",
	6669:  "irc",
	6881:  "bit-torrent",
	6999:  "bit-torrent",
	8080:  "http-proxy",
	11211: "memcached",
	26257: "cockroach-db",
	27017: "mongo-db",
}

var stateArray []string = []string{
	"",
	"ESTABLISHED",
	"SYN_SENT",
	"SYN_RECV",
	"FIN_WAIT1",
	"FIN_WAIT2",
	"TIME_WAIT",
	"CLOSE",
	"CLOSE_WAIT",
	"LAST_ACK",
	"LISTEN",
	"CLOSING",
	"NEW_SYN_RECV",
}

func getPodName() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get pod name.")
		return ""
	}
	return hostname
}

func getPortTotalCount() int {
	portTotalCount := 1
	procFS, err := procfs.NewFS("/proc")
	if err != nil {
		log.Println("Failed to read /proc.")
		return portTotalCount
	}

	portsArr, err := procFS.SysctlStrings("net/ipv4/ip_local_port_range")
	if err != nil {
		log.Println("Failed to read local port range.")
		return portTotalCount
	}
	if len(portsArr) < 2 {
		log.Println("Incorrect format of local port range.")
		return portTotalCount
	}

	firstPort, err := strconv.Atoi(portsArr[0])
	if err != nil {
		log.Println("Failed to read first local port number.")
		return portTotalCount
	}
	lastPort, err := strconv.Atoi(portsArr[1])
	if err != nil {
		log.Println("Failed to read last local port number.")
		return portTotalCount
	}
	portTotalCount = lastPort - firstPort + 1
	return portTotalCount
}

func getPortUsed() map[string]map[string]int {
	portStatistics := map[string]map[string]int{}

	procFS, err := procfs.NewFS("/proc")
	if err != nil {
		log.Println("Failed to read /proc.")
		return portStatistics
	}

	netTCP, err := procFS.NetTCP()
	if err != nil {
		log.Println("Failed to get proc/net/tcp information.")
		return portStatistics
	}

	// group TCP connections by "remote addr" and "state"
	portStatistics["OTHER"] = make(map[string]int)
	for _, c := range netTCP {
		portState := stateArray[c.St]
		if c.RemPort < 32768 {
			portName := portNameMap[c.RemPort]
			if "" == portName {
				portName = "unknown"
			}
			remoteAddr := fmt.Sprintf("%s [%d:%s]", c.RemAddr, c.RemPort, portName)
			if portStatistics[remoteAddr] == nil {
				portStatistics[remoteAddr] = make(map[string]int)
			}
			portStatistics[remoteAddr][portState] += 1
		} else { // temporary remote ports
			portStatistics["OTHER"][portState] += 1
		}
	}

	return portStatistics
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	podName := getPodName()

	// total port count
	portTotalCount := getPortTotalCount()
	portTotalOutputFormat := `
# HELP port_total Total Local Port Count
# TYPE port_total gauge
port_total{pod_name="%s"} %d
# HELP port_used Used Local Port Count
# TYPE port_used gauge`
	output := fmt.Sprintf(portTotalOutputFormat, podName, portTotalCount)

	// used port count for every remote addr
	portUsedStatistics := getPortUsed()
	portUsedCount := 0
	portUsedOutputFormat := `
port_used{pod_name="%s",remote_addr="%s",state="%s"} %d`
	for remoteAddr, m := range portUsedStatistics {
		for state, v := range m {
			portUsedCount += v
			output += fmt.Sprintf(portUsedOutputFormat, podName, remoteAddr, state, v)
		}
	}

	// port usage percentage
	portUsage := float32(portUsedCount) * 100 / float32(portTotalCount)
	portUsageOutputFormat := `
# HELP port_usage Local Port Usage Percentage
# TYPE port_usage gauge
port_usage{pod_name="%s"} %f`
	output += fmt.Sprintf(portUsageOutputFormat, podName, portUsage)

	w.Write([]byte(output))
}

func main() {
	port := os.Getenv("METRICS_SIDECAR_PORT")
	if "" == port {
		port = "9999"
	}

	http.HandleFunc("/metrics", metricsHandler)
	http.ListenAndServe("0.0.0.0:"+port, nil)
}
