### Tender

API Proxy for on Nitrous.IO zone. This is necessary because we want to expose
these API endpoints to end-user without exposing our authentication method on
the host (which is TLS). Tender's services acts as brokers, which speak
with Nitrous.IO public SSL certs to end-user, and speak Nitrous.IO private
certs with each host

This should be deployed once ( or many ) per region, and resolved by DNS (compass). For
example:

```
GET host.nitrousproxy.com/logs
----> DNS (compass)
----> host.nitrousproxy-region.com (tender server based on host's region)
----> Getting the address of the host
----> Proxy to the HTTP logs services on the host
```
