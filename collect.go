package main

import (
	"fmt"

	"github.com/prometheus/procfs"
)

// groupConnections groups TCP connections by "remote addr" and "state".
// Connections whose remote port is a temporary port are counted together as OTHER.
func groupConnections(conns procfs.NetTCP) map[string]map[string]int {
	portStatistics := map[string]map[string]int{}
	portStatistics["OTHER"] = make(map[string]int)
	for _, c := range conns {
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
