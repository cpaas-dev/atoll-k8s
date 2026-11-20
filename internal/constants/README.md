## BASH

- init.sh:  cd /var/lib/atoll/data/<clusterName>/scripts && bash init.sh $criData $registryDomain $registryPort $registryUsername $registryPassword
- init-registry.sh: cd /var/lib/atoll/data/<clusterName>/scripts && bash init-registry.sh $registryPort $registryData $registryConfig
- clean.sh: cd /var/lib/atoll/data/<clusterName>/scripts && bash clean.sh $criData
- clean-registry.sh: cd /var/lib/atoll/data/<clusterName>/scripts && bash clean-registry.sh $registryData $registryConfig
- auth.sh: cd /var/lib/atoll/data/<clusterName>/scripts && bash auth.sh
- check.sh: cd /var/lib/atoll/data/<clusterName>/scripts && bash check.sh

## Data

- logdir:  /var/lib/atoll/log
- tempdir: /var/lib/atoll/temp
- homedir: /var/lib/atoll/data/<clusterName>/
- datadir: /var/lib/atoll/data/<clusterName>/kube
- pkidir:  /var/lib/atoll/data/<clusterName>/pki
- etcdir:  /var/lib/atoll/data/<clusterName>/etc
- packagedir:  /var/lib/atoll/data/<clusterName>/package
- registryedir:  /var/lib/atoll/data/<clusterName>/registry
- kubeadm.conf:  /var/lib/atoll/data/<clusterName>/etc/admin.conf
- manifests: /var/lib/atoll/data/<clusterName>/kube/manifests
- charts: /var/lib/atoll/data/<clusterName>/kube/charts
- registry.yml: /var/lib/atoll/data/<clusterName>/kube/etc/registry.yml
- Kubeadmfile: /var/lib/atoll/data/<clusterName>/kube/etc/Kubeadmfile
- default.json: /var/lib/atoll/data/<clusterName>/kube/scripts/default.json
- scripts: /var/lib/atoll/data/<clusterName>/kube/scripts
- atollctl: /var/lib/atoll/data/<clusterName>/kube/opt/atollctl

## Work

- clusterfile: ~/.atoll/<clusterName>/Clusterfile
