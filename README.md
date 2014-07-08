### Tender

API Proxy for on Nitrous.IO zone. This is necessary because we want to expose
these API endpoints to end-user without exposing our authentication method on
the host (which is TLS). Tender's services acts as brokers, which speak
with Nitrous.IO public SSL certs to end-user, and speak Nitrous.IO zones'
specific certs with each zone.

This should be deployed once ( or many ) per region, and resolved by DNS. For
example:

```
GET container.host.nitrousapp.com/logs
----> DNS
----> 55.44.33.22 ( tender server)
----> Getting the address of the host
----> proxy to the HTTP logs services on the host
```
