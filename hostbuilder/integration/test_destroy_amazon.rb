#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

id = ARGV[0]
ip_address = ARGV[1]

params = {
  id: id,
  ip_address: ip_address,
  region_id: "us-west-2"
}

resp = client.post "http://localhost:6666/hosts/delete", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
