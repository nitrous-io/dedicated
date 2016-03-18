## Nitrous Web App

This Rails project is used for asset compilation and client-side javascript development. In production, the assets are served by the go app instead.


## Development

In development, you'd want Rails to be serving the assets (js, css) so that you don't have to recompile between reloads.

Use the following Nginx config so that API and Websocket calls are proxied to the Go app, and the assets are handled by the Rails app. Nginx also handles SSL termination.

```
user www-data;
worker_processes 4;
pid /run/nginx.pid;

events {
  worker_connections 768;
  # multi_accept on;
}

http {
  include /etc/nginx/mime.types;
  default_type application/octet-stream;

  access_log /var/log/nginx/access.log;
  error_log /var/log/nginx/error.log;

  sendfile on;

  map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
  }

  server {
    listen 8080 ssl;

    server_name pro.n2odev.com;
    ssl_certificate /home/vagrant/workspace/nitrous/dev/n2odev.crt;
    ssl_certificate_key /home/vagrant/workspace/nitrous/dev/n2odev.key;

    keepalive_timeout 70;

    location / {
      proxy_pass http://127.0.0.1:3000;
    }

    location /api/v0/ {
      proxy_pass http://127.0.0.1:3333;
      proxy_http_version 1.1;
      proxy_set_header Upgrade $http_upgrade;
      proxy_set_header Connection $connection_upgrade;
    }
  }
}
```
