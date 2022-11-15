# What is Metrics-Sidecar

`Metrics-Sidecar` is a way to monitor local port usage of containers:
- How many local ports are inuse.
- How many local ports can be used totally.

`Metrics-Sidecar` runs as an extra container of business pods. Since all containers share the same network namespace in one pod, so we can get the business network information from the `Metrics-Sidecar` container. In addition, we need expose a API `/metrics` so that `Prometheus` can scrape metrics data from `Metrics-Sidecar`.

In order to avoid impact on the business container, `Metrics-Sidecar` container NEVER exit even if it fails to get metrics, we can see those error information in its log.

# How to Use Metrics-Sidecar

Suppose that we want to monitor the port usage of `manju` container, we should perform the following steps.

## Add a Metrics Container to Target Business Pod

Edit `manju` deployment, add an extra container using image `metrics-sidecar` which is built by this project.
Please do not set any health check probes, otherwise its health status may affect the status of business container.

```
      containers:
      - image: asia.gcr.io/smartcart-stagingization/metrics-sidecar:develop
        imagePullPolicy: Always
        name: metrics
        env:
          name: METRICS_SIDECAR_PORT
          value: "9999"
        ports:
        - containerPort: 9999
        resources:
          limits:
            cpu: 250m
            memory: 128Mi
          requests:
            cpu: 250m
            memory: 128Mi
```

## Create a New Service for Metrics-Sidecar

As following, `namespace` and `selector` should be same as target business service `manju`, `targetPort` is the port of metrics service, `annotations` are necessary so that `prometheus` can scrape data from here.

```
apiVersion: v1
kind: Service
metadata:
  annotations:
    prometheus.io/path: /metrics
    prometheus.io/port: "9999"
    prometheus.io/scrape: "true"
  labels:
    app.kubernetes.io/name: manju
  name: manju-metrics
  namespace: default
spec:
  ports:
  - name: metrics
    port: 9999
    protocol: TCP
    targetPort: 9999
  selector:
    app.kubernetes.io/instance: manju
    app.kubernetes.io/name: manju
  type: ClusterIP
```

# How the Metrics Data Looks Like

The data format complies with the requirements of `prometheus`.

```
# HELP port_used Used Local Port Count
# TYPE port_used gauge
port_used{pod_name="manju-7744b786fc-jvftc"} 345
# HELP port_total Total Local Port Count
# TYPE port_total gauge
port_total{pod_name="manju-7744b786fc-jvftc"} 28888
```
