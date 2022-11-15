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

var podName string
var portTotalCount int

func getPodName() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get pod name.")
		return
	}
	podName = hostname
}

func getPortTotalCount() {
	procFS, err := procfs.NewFS("/proc")
	if err != nil {
		log.Println("Failed to read /proc.")
		return
	}

	portsArr, err := procFS.SysctlStrings("net/ipv4/ip_local_port_range")
	if err != nil {
		log.Println("Failed to read local port range.")
		return
	}
	if len(portsArr) < 2 {
		log.Println("Incorrect format of local port range.")
		return
	}

	firstPort, err := strconv.Atoi(portsArr[0])
	if err != nil {
		log.Println("Failed to read first local port number.")
		return
	}
	lastPort, err := strconv.Atoi(portsArr[1])
	if err != nil {
		log.Println("Failed to read last local port number.")
		return
	}
	portTotalCount = lastPort - firstPort + 1
}

func getPortUsedCount() int {
	procFS, err := procfs.NewFS("/proc")
	if err != nil {
		log.Println("Failed to read /proc.")
		return 0
	}

	netTCP, err := procFS.NetTCP()
	if err != nil {
		log.Println("Failed to get proc/net/tcp information.")
		return 0
	}

	return len(netTCP)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	portUsedCount := getPortUsedCount()

	outputFormat := `# HELP port_used Used Local Port Count
# TYPE port_used gauge
port_used{pod_name="%s"} %d
# HELP port_total Total Local Port Count
# TYPE port_total gauge
port_total{pod_name="%s"} %d`

	output := fmt.Sprintf(outputFormat, podName, portUsedCount, podName, portTotalCount)

	w.Write([]byte(output))
}

func main() {
	port := os.Getenv("METRICS_SIDECAR_PORT")
	if "" == port {
		port = "9999"
	}
	log.Println("Metrics server start...")
	http.HandleFunc("/metrics", metricsHandler)
	http.ListenAndServe("0.0.0.0:" + port, nil)
}

func init() {
	getPodName()
	getPortTotalCount()
}