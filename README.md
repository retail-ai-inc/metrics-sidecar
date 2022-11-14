# How to Use Metrics-Sidecar

Metrics-Sidecar is a way to monitor local port usage of containers.
Suppose that we want to monitor the port usage of `manju` container, we should perform the following steps.

## Add a Metrics Container to Target Application Pod

Edit `manju` deployment, add an extra container using image `metrics-sidecar` which is built by this project.

```
      containers:
      - image: asia.gcr.io/smartcart-stagingization/metrics-sidecar:develop
        imagePullPolicy: Always
        name: metrics
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

In this new container, there is a mini web server exposing only one API `/metrics`.

## Create a New Service for Metrics-Sidecar

As following, `namespace` and `selector` should be same as target application service `manju`, `targetPort` is the port of metrics service, `annotations` are necessary so that `prometheus` can scrape data from here.

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

```
# HELP port_used Used Local Port Count
# TYPE port_used gauge
port_used{pod_name="manju-7744b786fc-jvftc"} 345
# HELP port_total Total Local Port Count
# TYPE port_total gauge
port_total{pod_name="manju-7744b786fc-jvftc"} 28888
```

# Where the Data Comes From

`port_used` comes from file `/proc/net/tcp`, `port_total` comes from file `/proc/sys/net/ipv4/ip_local_port_range`.
