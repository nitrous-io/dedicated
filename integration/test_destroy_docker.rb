#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new

id = ARGV[0]

params = {
  id: id,
}

resp = client.post "http://localhost:6666/hosts/delete", {
  provider: "docker",
  webhook: "http://127.0.0.1:4567/webhook",
  config: JSON.generate(params)
}
p resp
