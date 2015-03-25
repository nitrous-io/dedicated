#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

id = ARGV[0]
ip_address = ARGV[1]
disable_eip = ARGV[2] == "true"

params = {
  id: id,
  ip_address: ip_address,
  region_id: "us-west-2",
  disable_eip: disable_eip
}

resp = client.post "http://localhost:6666/hosts/delete", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
