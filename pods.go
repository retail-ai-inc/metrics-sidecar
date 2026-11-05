package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

type podInfo struct {
	Namespace string
	Name      string
	App       string
}

// podCache keeps the list of Running Pods scheduled on this node, keyed by Pod UID.
// It talks to the API server directly with the in-cluster ServiceAccount
// instead of pulling in client-go.
type podCache struct {
	nodeName string
	selector string
	apiURL   string
	client   *http.Client

	mu   sync.RWMutex
	pods map[string]podInfo
}

func newPodCache(nodeName, selector string) (*podCache, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if "" == host || "" == port {
		return nil, fmt.Errorf("not running in a Kubernetes cluster: KUBERNETES_SERVICE_HOST/PORT not set")
	}
	ca, err := os.ReadFile(serviceAccountDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read ServiceAccount CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to parse ServiceAccount CA")
	}

	return &podCache{
		nodeName: nodeName,
		selector: selector,
		apiURL:   "https://" + net.JoinHostPort(host, port),
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		},
		pods: map[string]podInfo{},
	}, nil
}

func (c *podCache) run(interval time.Duration) {
	for {
		pods, err := c.list()
		if err != nil {
			// keep serving the last known list
			log.Println("Failed to list pods:", err)
		} else {
			c.mu.Lock()
			c.pods = pods
			c.mu.Unlock()
		}
		time.Sleep(interval)
	}
}

func (c *podCache) snapshot() map[string]podInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pods := make(map[string]podInfo, len(c.pods))
	for uid, p := range c.pods {
		pods[uid] = p
	}
	return pods
}

type podList struct {
	Items []struct {
		Metadata struct {
			UID       string            `json:"uid"`
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			HostNetwork bool `json:"hostNetwork"`
		} `json:"spec"`
	} `json:"items"`
}

func (c *podCache) list() (map[string]podInfo, error) {
	q := url.Values{}
	q.Set("fieldSelector", "spec.nodeName="+c.nodeName+",status.phase=Running")
	if "" != c.selector {
		q.Set("labelSelector", c.selector)
	}
	req, err := http.NewRequest(http.MethodGet, c.apiURL+"/api/v1/pods?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// re-read every time: projected ServiceAccount tokens are rotated by kubelet
	token, err := os.ReadFile(serviceAccountDir + "/token")
	if err != nil {
		return nil, fmt.Errorf("failed to read ServiceAccount token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list pods: %s", resp.Status)
	}

	var list podList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode pod list: %w", err)
	}

	pods := make(map[string]podInfo, len(list.Items))
	for _, item := range list.Items {
		// hostNetwork Pods share the node's network namespace, counting them
		// would report the whole node's connections once per such Pod.
		if item.Spec.HostNetwork {
			continue
		}
		pods[item.Metadata.UID] = podInfo{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			App:       appLabel(item.Metadata.Labels),
		}
	}
	return pods, nil
}

func appLabel(labels map[string]string) string {
	for _, k := range []string{"app.kubernetes.io/name", "app", "app.kubernetes.io/instance"} {
		if v := labels[k]; "" != v {
			return v
		}
	}
	return ""
}
