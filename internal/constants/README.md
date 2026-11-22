## Paths

Defaults; override with `ATOLL_RUNTIME_ROOT` and `ATOLL_DATA_ROOT` (see `atoll env`).

Local, where `atoll` runs:

- Clusterfile: ~/.atoll/<clusterName>/Clusterfile
- logs: ~/.atoll/logs
- image store: ~/.atoll/images, one OCI image layout filled by `atoll pull` and `atoll run <archive>`
- rootfs exports: ~/.atoll/<clusterName>/mounts/<name>, the `/cpaas` dir of each cluster image, recorded in `<name>.json`
- pki: ~/.atoll/<clusterName>/pki
- admin.conf: ~/.atoll/<clusterName>/etc/admin.conf
- tmp: ~/.atoll/<clusterName>/tmp

Hosts, `<rootfs>` is /var/lib/atoll/data/<clusterName>/rootfs, the content of `/cpaas` in the cluster image:

- configs: /var/lib/atoll/data/<clusterName>/etc
- etc: <rootfs>/etc
- scripts: <rootfs>/scripts
- manifests: <rootfs>/manifests
- charts: <rootfs>/charts
- bin: <rootfs>/bin
- statics: <rootfs>/statics
- atollctl: <rootfs>/opt/atollctl
- registry.yml: <rootfs>/etc/registry.yml

## Scripts

Scripts run on hosts as `cd <rootfs>/scripts && <env> bash <script>`. `<script>` is the cluster image label of the same name (it may carry arguments, e.g. `init-registry.sh $registryData $registryConfig`), or `<name>.sh` when the label is not set. `<env>` is the cluster image ENV merged with the Clusterfile `spec.env` and host env.

- check: every host, preflight
- init, clean: every host
- init-registry, clean-registry: registry hosts
- init-cri, clean-cri: only when the label is set

`*.tmpl` files under `<rootfs>/etc`, `<rootfs>/scripts` and `<rootfs>/manifests` are rendered on every host by `atollctl render` (Go template, the env above as top-level keys plus a `toJson` func) before any script runs.

## Registry

Registry hosts run zot v2.1.23 as systemd service `zot` (defaults: domain atoll.hub, port 5000). atoll reads registry ip, domain, port, username, password and data dir from `<rootfs>/etc/registry.yml`; its `data` must equal `registryData`. atoll writes the bcrypt htpasswd to `<rootfs>/etc/registry_htpasswd` before running init-registry.sh, waits for the port afterwards, and fills the registry by pushing images through the registry API. Nothing writes into the data dir directly. `atoll registry passwd` rewrites /etc/registry/registry_htpasswd on registry hosts and runs `systemctl restart zot`.

The cluster image must provide:

- env: `registryData` (default /var/lib/registry), `registryPort` (5000), `registryConfig` (must be /etc/registry), `registryDomain` (atoll.hub), `registryUsername`, `registryPassword`
- `bin/zot`: zot v2.1.23 binary for the image architecture
- `etc/zot.service`: unit running `/usr/bin/zot serve /etc/registry/registry_config.json`
- `etc/registry_config.json.tmpl`: zot config, `storage.gc` must stay false (GC deletes untagged manifests that are referenced by digest):

```json
{
  "distSpecVersion": "1.1.1",
  "storage": {"rootDirectory": "{{ .registryData }}", "dedupe": true, "gc": false},
  "http": {"address": "0.0.0.0", "port": "{{ .registryPort }}", "compat": ["docker2s2"],
           "auth": {"htpasswd": {"path": "{{ .registryConfig }}/registry_htpasswd"}}},
  "log": {"level": "info"}
}
```

init-registry.sh must:

1. create `$registryData` as a real directory (remove a symlink at that path first); it is zot's `storage.rootDirectory`, not part of the rootfs
2. install `../etc/registry_config.json` and `../etc/registry_htpasswd` into `$registryConfig`, `../bin/zot` to /usr/bin/zot and `../etc/zot.service` to /etc/systemd/system/
3. run `zot verify $registryConfig/registry_config.json` and fail on error
4. stop and disable `registry.service` if present
5. `systemctl daemon-reload && systemctl enable --now zot`

clean-registry.sh must `systemctl disable --now zot`, then remove /etc/systemd/system/zot.service, /usr/bin/zot, `$registryConfig` and `$registryData`. The data dir is outside the rootfs, so removing the rootfs does not remove it.
