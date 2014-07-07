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
  config: JSON.generate(params)
}
p resp
