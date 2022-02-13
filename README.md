# helm-s3-proxy
A HTTP compatible read proxy for the [helm-s3](https://github.com/hypnoglow/helm-s3) plugin.

# Problem

You are currently using the helm s3 plugin to store your helm charts, but now need to migrate to a helm setup that requires pulling charts from helm's native HTTP.  For us, this was the migration to [flux 2.0](https://github.com/fluxcd/flux2).  The solution is to install and use this proxy, which allows both options at the same time.

# How it works

In our setup, we have dozens of helm repositories inside our s3 bucket: one per chart.  This is because the index.yaml helm uses grows too large for all our helm charts for our current CI/CD system.  Our S3 bucket looks something like this (but imagine dozens of chart names):

```
s3://company-bucket/chart1/index.yaml
s3://company-bucket/chart1/chart1-0.0.1.tgz
s3://company-bucket/chart2/index.yaml
s3://company-bucket/chart2/chart2-0.0.1.tgz
s3://company-bucket/chart2/chart2-0.0.2.tgz
```

One of the index files, s3://company-bucket/chart2/index.yaml, may look like this

```yaml
apiVersion: v1
entries:
  chart2:
  - apiVersion: v2
    name: chart2
    urls:
    - s3://company-bucket/chart2/chart2-0.0.1.tgz
    version: 0.0.1
  - apiVersion: v2
    name: chart2
    type: application
    urls:
    - s3://company-bucket/chart2/chart2-0.0.2.tgz
    version: 0.0.2
generated: "2021-08-15T13:22:44.39851397Z"
```

The proxy will expose an endpoint, /chart2/index.yaml, that returns the following file (notice how it also changes the `urls` part)

```yaml
apiVersion: v1
entries:
  chart2:
  - apiVersion: v2
    name: chart2
    urls:
    - http://helm-s3-proxy.helm-s3-proxy.svc.cluster.local/chart2/chart2-0.0.1.tgz
    version: 0.0.1
  - apiVersion: v2
    name: chart2
    type: application
    urls:
    - http://helm-s3-proxy.helm-s3-proxy.svc.cluster.local/chart2/chart2-0.0.2.tgz
    version: 0.0.2
generated: "2021-08-15T13:22:44.39851397Z"
```

We then configure flux2 to fetch helm charts from the proxy:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta1
kind: HelmRepository
metadata:
  name: chart2
spec:
  interval: 1m0s
  url: http://helm-s3-proxy.helm-s3-proxy.svc.cluster.local/chart2
```

# Pre install

You will need an IAM role that can read files from your S3 bucket.  Pass this role to the helm-s3-proxy service account.  For an example, check out install below.

# Install

Here is our flux2 configuration for installing the helm-s3-proxy

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta1
kind: HelmRepository
metadata:
  name: helm-s3-proxy
  namespace: helm-s3-proxy
spec:
  interval: 1m0s
  url: https://cresta.github.io/helm-s3-proxy/
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: helm-s3-proxy
  namespace: helm-s3-proxy
spec:
  chart:
    spec:
      chart: helm-s3-proxy
      sourceRef:
        kind: HelmRepository
        name: helm-s3-proxy
      version: 0.1.7
  interval: 1m0s
  releaseName: helm-s3-proxy
  values:
    s3_bucket: our-helm-bucket-name
    replace_http_path: http://helm-s3-proxy.helm-s3-proxy.svc.cluster.local
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::${cluster_account_id}:role/${cluster_env}-eks-helm-bucket-read-role
```

# Implementation notes

The proxy will cache the contents of index.yaml and their E-tag: requests to S3 return NotModified if the e-tag is the same.  This allows us to speed up the more frequent index.yaml fetches.
