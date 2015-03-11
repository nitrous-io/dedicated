#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

params = {
  ignore_region: true
}

resp = client.get "http://localhost:6666/hosts", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
