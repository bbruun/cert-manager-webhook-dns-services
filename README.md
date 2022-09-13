<p align="center">
  <img src="https://raw.githubusercontent.com/cert-manager/cert-manager/d53c0b9270f8cd90d908460d69502694e1838f5f/logo/logo-small.png" height="256" width="256" alt="cert-manager project logo" />
</p>

# ACME webhook for DNS.Services

This is a webhook for [Cert Manager](https://cert-manager.io) for the DNS hosting service [DNS.Services](https://dns.services).

This is the k8s version of the [acme.sh](https://github.com/acmesh-official/acme.sh) DNS-01 provider script [dns_dnsservices.sh](https://github.com/acmesh-official/acme.sh/blob/master/dnsapi/dns_dnsservices.sh) that is released but is still undergoing "initial multi zone testing".

# Installation

As of this release the webhook is still in testing phase, so the installation is manual.

## First Install Helm or Kubectl

There are 2 ways to install Cert Manager - helm or kubectl.
There is currently only only one way to install this webhook - kubectl.

So make sure you have both [Helm](https://helm.sh/) and [kubectl](https://kubernetes.io/docs/tasks/tools/) installed as you'll need at least one of them.
## Installing Cert Manager

Follow the Cert Manager installation guide for [helm]](https://cert-manager.io/docs/installation/helm/) or [kubectl](https://cert-manager.io/docs/installation/kubectl/) eg 

```shell
helm repo add jetstack https://charts.jetstack.io

helm repo update

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.9.1/cert-manager.crds.yaml

helm install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.9.1
```

Note: If you, like me the developer, is running a home lab k8s cluster on k3s with an internal network and host your own DNS zone in a split setup aka an internal DNS server solely for internal IPs and user a public DNS provide (such as DNS.Services) then add the following to the `helm install ...` command above. If you do not then you'll possibly end up in evil loop as your internal DNS is authoriative the domain on DNS.Services where you created the TXT record which isn't on your internal DNS server. The extra parameter to the `helm install ...` comamnd is: 

```shell
heml install \
  ... \
  --set 'extraArgs={--dns01-recursive-nameservers-only,--dns01-recursive-nameservers=195.242.131.6:53}'
```
(the IP is for ns1.dns.services)
[Taken from CM's DNS resolver troubleshooting guide](https://cert-manager.io/v1.6-docs/configuration/acme/dns01/). You can change the DNS provider (for some reason CM complains about more than one as described in the documentation). **Remember the backslash "\" on the line above when/if you add it - it means "command continues on next line**.

## Install this DNS.Services Webhook

As of this alpha release most of the building and testing is done internally with my own registry, but I'll try to keep the following up2date

`docker.io/bbruun/dns-services-webhook:latest`

This means that you 
1. need to clone this repo
2. change to the `dns-services-webhook/deploy` directory
3. run ` helm install dns-services-webhook dns-services-webhook --namespace cert-manager`

## Configuration

You need to setup a Issuer or ClusterIssuer for CM to work.


Example: 
```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencryptOfYourChoice
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
    - dns01:
        webhook:
          config:
            Email: your@example.com
            Password: yourDNSServicesAPIPassword
            Host: https://dns.services/api
          groupName: dns.services.cert-manager.io
          solverName: dns.services.cert-manager.io
```
The `Issuer` is just like the above, but is `kind: Issuer` and needs to be applied to the `cert-manager` namespace as it is not a ClusterIssuer.

## Configuring a certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: name-of-certificate
spec:
  secretName: your-secret-name-for-the-example.com-certificate
  commonName: example.com
  dnsNames:
  - example.com
  - '*.example.com'
  issuerRef:
    name: IssuerOrClusterIssuerName_eg_the_above_letsencryptOfYourChoice
    kind: ClusterIssuer
```

Apply this to the namespace where it is to be used


# Using it all

## Issue a certificate you use by your self in a secret

Create a Certificate as described above and add it the a namespace eg


```shell
kubectl -n awesomeNamespace apply -f <nameOfCertificateYamlFile>
```

After a couple minutes the secret `secretName` will be updated and you can use it as you like

## Using it with an Ingress Controller

Cert Manager needs some annotations on (all) your Ingress/IngressRoutes to be able to scan, issue and create the secret needed to use the certificate.

The annotataion is as follows:

```ỳaml
  annotations:
    kubernetes.io/ingress.class: $INGRESS_CONTROLLER
    cert-manager.io/cluster-issuer: NAME_OF_Issuer_OR_ClusterIssuer
```

Your INGRESS_CONTROLLER is eg `nginx` or `nginx-ingress` or `traefik` or .. which ever Ingress Controller you have/use.

# Developer and testing notes

Requirements:
* GIT
* Go 1.18+
* Docker or [podman](https://podman.io/) (a great alternative to Docker)
* A https://DNS.Services account
* a Kubernetes cluster you control

# Issues you may encounter

Besides all the issues for [Cert Manager](https://github.com/cert-manager/cert-manager/issues) it self then I've only observed 2

# Cert Managers CNAME issues

I've found that the default wildcard CNAME entry `*.example.com` pointing to the root zone A record makes Cert Manager loop and state the the _acme-challenge DNS record has not yet been propergated.
If you add a A record named `lb.example.com` that is your "default" A record and setup the wildcard CNAME `*.example.com` then it works.

This seems to be a somewhat wellknown issue with Cert Manager and this is not the only DNS provider where **Cert Manager has this issue**.

## Issue #1: DNS resolution

When Cert Manager creates a certificate it does internal DNS resolution for the _acme-challenge.example.com CNAME entry.

If you happen to run into the logs for Cert Manager just stating something like the following:

`"error"="DNS record for \"subdom.example.com\" not yet propagated"`

Then you need to edit the Cert Manager Deployment and add the above mentioned extraArgs (the last two below in the snippet) and do a restart of the cert-manager deployment so one of two ways to fix this:

### Fix manually - not helm upgradable afterwards

1. Edit the Deployment `kubectl -n cert-manager edit deployment cert-manager` and add the two --dns01... parameters
```yaml
spec:
  template:
    spec:
      containers:
      - args:
        - --v=2
        - --cluster-resource-namespace=$(POD_NAMESPACE)
        - --leader-election-namespace=kube-system
        - --dns01-recursive-nameservers-only
        - --dns01-recursive-nameservers="195.242.131.6:53"
```
2. Restart Cert Manager `kubectl -n cert-manager rollout restart deployment cert-manager` and wait a few minutes for it to restart and be ready.

### Fix correctly


1. Since you have the YAML for your Issuer/ClusterIssuer and secret for the DNS Services account, then uninstall first this webhook and then cert-manager:

1. Change to the deploy/ directory in this repo and uninstall this webhook
```shell
helm uninstall dns-services-webhook --namespace cert-manager
```
2. Uninstall Cert Manager 
```shell
helm uninstall cert-manager --namespace=cert-manager
```
3. Re-install Cert Manager but add the extraArgs to the installation 
```shell
helm install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.9.1 
  --set 'extraArgs={--dns01-recursive-nameservers-only,--dns01-recursive-nameservers=195.242.131.6:53}'
```
1. Re-install this webhook as described above and apply your Issuer/ClusterIssuer configurations and you should be good to go for later Helm upgrades of Cert Manager.


# Forking and developing etc.

You are always welcome to create a ticket/issue if you have issues, but please do check that the issue is not
* your Certificate configuration
* your ClusterIssuer configuration
* your Issuer configuration
* a well know [Cert Manager issue](https://github.com/cert-manager/cert-manager/issues)

Also if you fork the repo then please do so :-)

## A note on running the test suite

As per https://github.com/cert-manager/webhook-example then please do make sure that any pull-request you make works...

1. Make a copy of the config.json.sample file `cp testdata/dns-services/config.json.sample testdata/dns-services/config.json`
2. Make a copy of the dns-service.yaml.sample file `cp testdata/dns-services/dns-services.yaml.sample testdata/dns-services/dns-services.yaml`
3. Update the `dns-service.yaml` **using the base64** version of your username and password for https://DNS.Services (no TOTP needed).
4. Run the test 
```shell
TEST_ZONE_NAME=example.com. make test
```

Due to limits in the API in regards to amount of logins per time then do not run the test too many times in a row or when setting up new certificates (TLS configuartions in your Ingress/Gateway's). I would guestimate 1-3 certificates per 5min would be the limit - but just in case your certificate is not created, then delete the TLS entry in your Ingress/Gateway, wait 5-10min and add it again.
