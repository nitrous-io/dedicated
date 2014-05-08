#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new

id = ARGV[0]
name = ARGV[1]

unless id
  p "missing id"
  exit 1
end

unless name
  p "missing name"
  exit 1
end

params = {
  id: id,
  name: name
}

resp = client.post "http://localhost:6666/host/delete", {
  provider: "digitalocean",
  webhook: "http://127.0.0.1:4567/webhook",
  config: JSON.generate(params)
}
p resp
